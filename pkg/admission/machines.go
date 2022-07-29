/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package admission

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/crypto/ssh"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	controllerutil "github.com/kubermatic/machine-controller/pkg/controller/util"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog"
)

// BypassSpecNoModificationRequirementAnnotation is used to bypass the "no machine.spec modification" allowed
// restriction from the webhook in order to change the spec in some special cases, e.G. for the migration of
// the `providerConfig` field to `providerSpec`.
const BypassSpecNoModificationRequirementAnnotation = "kubermatic.io/bypass-no-spec-mutation-requirement"

func (ad *admissionData) mutateMachines(ctx context.Context, ar admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	machine := clusterv1alpha1.Machine{}
	if err := json.Unmarshal(ar.Object.Raw, &machine); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	machineOriginal := machine.DeepCopy()
	klog.V(3).Infof("Defaulting and validating machine %s/%s", machine.Namespace, machine.Name)

	// Mutating .Spec is never allowed
	// Only hidden exception: the machine-controller may set the .Spec.Name to .Metadata.Name
	// because otherwise it can never add the delete finalizer as it internally defaults the Name
	// as well, since on the CREATE request for machines, there is only Metadata.GenerateName set
	// so we can't default it initially.
	if ar.Operation == admissionv1.Update {
		oldMachine := clusterv1alpha1.Machine{}
		if err := json.Unmarshal(ar.OldObject.Raw, &oldMachine); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OldObject: %w", err)
		}
		if oldMachine.Spec.Name != machine.Spec.Name && machine.Spec.Name == machine.Name {
			oldMachine.Spec.Name = machine.Spec.Name
		}
		// Allow mutation when:
		// * machine has the `MigrationBypassSpecNoModificationRequirementAnnotation` annotation (used for type migration)
		bypassValidationForMigration := machine.Annotations[BypassSpecNoModificationRequirementAnnotation] == "true"
		if !bypassValidationForMigration {
			if equal := apiequality.Semantic.DeepEqual(machine.Spec, oldMachine.Spec); !equal {
				return nil, fmt.Errorf("machine.spec is immutable")
			}
		}
	}
	// Delete the `BypassSpecNoModificationRequirementAnnotation` annotation, it should be valid only once.
	delete(machine.Annotations, BypassSpecNoModificationRequirementAnnotation)

	// Default name
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}

	// Default and verify .Spec on CREATE only, its expensive and not required to do it on UPDATE
	// as we disallow .Spec changes anyways.
	if ar.Operation == admissionv1.Create {
		if err := ad.defaultAndValidateMachineSpec(ctx, &machine.Spec); err != nil {
			return nil, err
		}

		common.SetKubeletFeatureGates(&machine, ad.nodeSettings.KubeletFeatureGates)
		common.SetKubeletFlags(&machine, map[common.KubeletFlags]string{
			common.ExternalCloudProviderKubeletFlag: fmt.Sprintf("%t", ad.nodeSettings.ExternalCloudProvider),
		})
		providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
		if err != nil {
			return nil, err
		}
		common.SetOSLabel(&machine.Spec, string(providerConfig.OperatingSystem))
	}

	// Set LegacyMachineControllerUserDataLabel to false if OSM was used for managing the machine configuration.
	if ad.useOSM {
		if machine.Labels == nil {
			machine.Labels = make(map[string]string)
		}
		machine.Labels[controllerutil.LegacyMachineControllerUserDataLabel] = "false"
	}

	return createAdmissionResponse(machineOriginal, &machine)
}

func (ad *admissionData) defaultAndValidateMachineSpec(ctx context.Context, spec *clusterv1alpha1.MachineSpec) error {
	providerConfig, err := providerconfigtypes.GetConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to read machine.spec.providerSpec: %w", err)
	}

	// Packet has been renamed to Equinix Metal
	if providerConfig.CloudProvider == cloudProviderPacket {
		err = migrateToEquinixMetal(providerConfig)
		if err != nil {
			return fmt.Errorf("failed to migrate packet to equinix metal: %w", err)
		}
	}

	skg := providerconfig.NewConfigVarResolver(ctx, ad.workerClient)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %w", providerConfig.CloudProvider, err)
	}

	// Verify operating system.
	if _, err := ad.userDataManager.ForOS(providerConfig.OperatingSystem); err != nil {
		return fmt.Errorf("failed to get OS '%s': %w", providerConfig.OperatingSystem, err)
	}

	// Check kubelet version
	if spec.Versions.Kubelet == "" {
		return fmt.Errorf("Kubelet version must be set")
	}

	kubeletVer, err := semver.NewVersion(spec.Versions.Kubelet)
	if err != nil {
		return fmt.Errorf("failed to parse kubelet version: %w", err)
	}

	if !ad.constraints.Check(kubeletVer) {
		return fmt.Errorf("kubernetes version constraint didn't allow %q kubelet version", kubeletVer)
	}

	// Do not allow 1.24+ to use config source (dynamic kubelet configuration)
	constraint124, err := semver.NewConstraint(">= 1.24")
	if err != nil {
		return fmt.Errorf("failed to parse 1.24 constraint: %w", err)
	}

	if constraint124.Check(kubeletVer) {
		if spec.ConfigSource != nil {
			return fmt.Errorf("setting spec.ConfigSource is not allowed for kubelet version %q", kubeletVer)
		}
	}

	// Validate SSH keys
	if err := validatePublicKeys(providerConfig.SSHPublicKeys); err != nil {
		return fmt.Errorf("Invalid public keys specified: %w", err)
	}

	defaultedOperatingSystemSpec, err := providerconfig.DefaultOperatingSystemSpec(
		providerConfig.OperatingSystem,
		providerConfig.CloudProvider,
		providerConfig.OperatingSystemSpec,
		ad.useOSM,
	)
	if err != nil {
		return err
	}

	providerConfig.OperatingSystemSpec = defaultedOperatingSystemSpec
	spec.ProviderSpec.Value.Raw, err = json.Marshal(providerConfig)
	if err != nil {
		return fmt.Errorf("failed to json marshal machine.spec.providerSpec: %w", err)
	}

	defaultedSpec, err := prov.AddDefaults(*spec)
	if err != nil {
		return fmt.Errorf("failed to default machineSpec: %w", err)
	}
	spec = &defaultedSpec

	if err := prov.Validate(ctx, *spec); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

func validatePublicKeys(keys []string) error {
	for _, s := range keys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
		if err != nil {
			return fmt.Errorf("invalid public key %q: %w", s, err)
		}
	}

	return nil
}
