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
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/ssh"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog"
)

// BypassSpecNoModificationRequirementAnnotation is used to bypass the "no machine.spec modification" allowed
// restriction from the webhook in order to change the spec in some special cases, e.G. for the migration of
// the `providerConfig` field to `providerSpec`
const BypassSpecNoModificationRequirementAnnotation = "kubermatic.io/bypass-no-spec-mutation-requirement"

func (ad *admissionData) mutateMachines(ar admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {

	machine := clusterv1alpha1.Machine{}
	if err := json.Unmarshal(ar.Object.Raw, &machine); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %v", err)
	}
	machineOriginal := machine.DeepCopy()
	klog.V(3).Infof("Defaulting and validating machine %s/%s", machine.Namespace, machine.Name)

	// Mutating .Spec is never allowed
	// Only hidden exception: the machine-controller may set the .Spec.Name to .Metadata.Name
	// because otherwise it can never add the delete finalizer as it internally defaults the Name
	// as well, since on the CREATE request for machines, there is only Metadata.GenerateName set
	// so we can't default it initially
	if ar.Operation == admissionv1.Update {
		oldMachine := clusterv1alpha1.Machine{}
		if err := json.Unmarshal(ar.OldObject.Raw, &oldMachine); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OldObject: %v", err)
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
	// Delete the `BypassSpecNoModificationRequirementAnnotation` annotation, it should be valid only once
	delete(machine.Annotations, BypassSpecNoModificationRequirementAnnotation)

	// Default name
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}

	// Default and verify .Spec on CREATE only, its expensive and not required to do it on UPDATE
	// as we disallow .Spec changes anyways
	if ar.Operation == admissionv1.Create {
		if err := ad.defaultAndValidateMachineSpec(&machine.Spec); err != nil {
			return nil, err
		}
	}

	return createAdmissionResponse(machineOriginal, &machine)
}

func (ad *admissionData) defaultAndValidateMachineSpec(spec *clusterv1alpha1.MachineSpec) error {
	providerConfig, err := providerconfigtypes.GetConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to read machine.spec.providerSpec: %v", err)
	}
	skg := providerconfig.NewConfigVarResolver(ad.ctx, ad.client)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %v", providerConfig.CloudProvider, err)
	}

	// Verify operating system.
	if _, err := ad.userDataManager.ForOS(providerConfig.OperatingSystem); err != nil {
		return fmt.Errorf("failed to get OS '%s': %v", providerConfig.OperatingSystem, err)
	}

	// Check kubelet version
	if spec.Versions.Kubelet == "" {
		return fmt.Errorf("Kubelet version must be set")
	}

	// Validate SSH keys
	if err := validatePublicKeys(providerConfig.SSHPublicKeys); err != nil {
		return fmt.Errorf("Invalid public keys specified: %v", err)
	}

	defaultedSpec, err := prov.AddDefaults(*spec)
	if err != nil {
		return fmt.Errorf("failed to default machineSpec: %v", err)
	}
	spec = &defaultedSpec

	if err := prov.Validate(*spec); err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}

	return nil
}

func validatePublicKeys(keys []string) error {
	for _, s := range keys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
		if err != nil {
			return fmt.Errorf("invalid public key %q: %v", s, err)
		}
	}

	return nil
}
