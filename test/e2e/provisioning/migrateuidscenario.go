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

package provisioning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/klog"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
)

func verifyMigrateUID(kubeConfig, manifestPath string, parameters []string, timeout time.Duration) error {
	// prepare the manifest
	manifest, err := readAndModifyManifest(manifestPath, parameters)
	if err != nil {
		return fmt.Errorf("failed to prepare the manifest, due to: %v", err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return fmt.Errorf("error building kubeconfig: %v", err)
	}
	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		return fmt.Errorf("failed to construct ctrlruntimeclient: %v", err)
	}
	providerData := &cloudprovidertypes.ProviderData{
		Update: cloudprovidertypes.GetMachineUpdater(context.Background(), client),
		Client: client,
	}

	machineDeployment := &v1alpha1.MachineDeployment{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	if err := manifestDecoder.Decode(machineDeployment); err != nil {
		return fmt.Errorf("failed to decode manifest into MachineDeployment: %v", err)
	}
	machine := &v1alpha1.Machine{
		ObjectMeta: machineDeployment.Spec.Template.ObjectMeta,
		Spec:       machineDeployment.Spec.Template.Spec,
	}

	oldUID := types.UID(fmt.Sprintf("aaa-%s", machineDeployment.Name))
	newUID := types.UID(fmt.Sprintf("bbb-%s", machineDeployment.Name))
	machine.UID = oldUID
	machine.Name = machineDeployment.Name
	machine.Spec.Name = machine.Name

	providerSpec, err := providerconfig.GetConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to get provideSpec: %v", err)
	}
	fakeClient := fakectrlruntimeclient.NewFakeClient()
	skg := providerconfig.NewConfigVarResolver(context.Background(), fakeClient)
	prov, err := cloudprovider.ForProvider(providerSpec.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %v", providerSpec.CloudProvider, err)

	}
	defaultedSpec, err := prov.AddDefaults(machine.Spec)
	if err != nil {
		return fmt.Errorf("failed to add defaults: %v", err)
	}
	machine.Spec = defaultedSpec

	// Step 0: Create instance with old UID
	maxTries := 15
	for i := 0; i < maxTries; i++ {
		_, err := prov.Get(machine, providerData)
		if err != nil {
			if err != cloudprovidererrors.ErrInstanceNotFound {
				if i < maxTries-1 {
					time.Sleep(10 * time.Second)
					klog.V(4).Infof("failed to get machine %s before creating it on try %v with err=%v, will retry", machine.Name, i, err)
					continue
				}
				return fmt.Errorf("failed to get machine %s before creating it: %v", machine.Name, err)
			}
			_, err := prov.Create(machine, providerData, "#cloud-config\n")
			if err != nil {
				if i < maxTries-1 {
					time.Sleep(10 * time.Second)
					klog.V(4).Infof("failed to create machine %s on try %v with err=%v, will retry", machine.Name, i, err)
					continue
				}
				return fmt.Errorf("failed to create machine %s: %v", machine.Name, err)
			}
		}
		break
	}

	// Step 1: Verify we can successfully get the instance
	for i := 0; i < maxTries; i++ {
		if _, err := prov.Get(machine, providerData); err != nil {
			if i < maxTries-1 {
				klog.V(4).Infof("failed to get instance for machine %s before migrating on try %v with err=%v, will retry", machine.Name, i, err)
				time.Sleep(10 * time.Second)
				continue
			}
			return fmt.Errorf("failed to get machine %s after creating it: %v", machine.Name, err)
		}
		break
	}

	// Step 2: Migrate UID
	for i := 0; i < maxTries; i++ {
		if err := prov.MigrateUID(machine, newUID); err != nil {
			if i < maxTries-1 {
				time.Sleep(10 * time.Second)
				klog.V(4).Infof("failed to migrate UID for machine %s  on try %v with err=%v, will retry", machine.Name, i, err)
				continue
			}
			return fmt.Errorf("failed to migrate UID for machine %s: %v", machine.Name, err)
		}
		break
	}
	machine.UID = newUID

	// Step 3: Verify we can successfully get the instance with the new UID
	for i := 0; i < maxTries; i++ {
		if _, err := prov.Get(machine, providerData); err != nil {
			if i < maxTries-1 {
				time.Sleep(10 * time.Second)
				klog.V(4).Infof("failed to get instance for machine %s after migrating on try %v with err=%v, will retry", machine.Name, i, err)
				continue
			}
			return fmt.Errorf("failed to get machine %s after migrating UID: %v", machine.Name, err)
		}
		break
	}

	// Step 4: Delete the instance and then verify instance is gone
	for i := 0; i < maxTries; i++ {

		// Deletion part 0: Delete and continue on err if there are tries left
		done, err := prov.Cleanup(machine, providerData)
		if err != nil {
			if i < maxTries-1 {
				klog.V(4).Infof("Failed to delete machine %s on try %v with err=%v, will retry", machine.Name, i, err)
				time.Sleep(10 * time.Second)
				continue
			}
			return fmt.Errorf("failed to delete machine %s: %v", machine.Name, err)
		}
		if !done {
			// The deletion is async, thus we wait 10 seconds to recheck if its done
			time.Sleep(10 * time.Second)
			continue
		}

		// Deletion part 1: Get and continue if err != cloudprovidererrors.ErrInstanceNotFound if there are tries left
		_, err = prov.Get(machine, providerData)
		if err != nil && err == cloudprovidererrors.ErrInstanceNotFound {
			break
		}
		if i < maxTries-1 {
			klog.V(4).Infof("Get after deleting instance for machine %s did not return ErrInstanceNotFound but err=%v", machine.Name, err)
			// Wait a little, as some providers like AWS delete asynchronously
			time.Sleep(10 * time.Second)
			continue
		}
		return fmt.Errorf("expected ErrInstanceNotFound after deleting instance for machine %s, but got err=%v", machine.Name, err)
	}

	return nil
}
