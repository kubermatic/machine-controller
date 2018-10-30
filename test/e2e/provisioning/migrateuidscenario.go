package provisioning

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func verifyMigrateUID(_, manifestPath string, parameters []string, timeout time.Duration) error {
	// prepare the manifest
	manifest, err := readAndModifyManifest(manifestPath, parameters)
	if err != nil {
		return fmt.Errorf("failed to prepare the manifest, due to: %v", err)
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

	providerConfig, err := providerconfig.GetConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to get provider config: %v", err)
	}
	fakeClient := fake.NewSimpleClientset()
	skg := providerconfig.NewConfigVarResolver(fakeClient)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %v", providerConfig.CloudProvider, err)

	}
	defaultedSpec, _, err := prov.AddDefaults(machine.Spec)
	if err != nil {
		return fmt.Errorf("failed to add defaults: %v", err)
	}
	machine.Spec = defaultedSpec

	// Step 0: Create instance with old UID
	maxTries := 15
	for i := 0; i < maxTries; i++ {
		_, err := prov.Get(machine)
		if err != nil {
			if err != cloudprovidererrors.ErrInstanceNotFound {
				if i < maxTries-1 {
					time.Sleep(10 * time.Second)
					glog.V(4).Infof("failed to get machine %s before creating it on try %v with err=%v, will retry", machine.Name, i, err)
					continue
				}
				return fmt.Errorf("failed to get machine %s before creating it: %v", machine.Name, err)
			}
			_, err := prov.Create(machine, machineUpdater, "#cloud-config")
			if err != nil {
				if i < maxTries-1 {
					time.Sleep(10 * time.Second)
					glog.V(4).Infof("failed to create machine %s on try %v with err=%v, will retry", machine.Name, i, err)
					continue
				}
				return fmt.Errorf("failed to create machine %s: %v", machine.Name, err)
			}
		}
		break
	}

	// Step 1: Verify we can successfully get the instance
	for i := 0; i < maxTries; i++ {
		if _, err := prov.Get(machine); err != nil {
			if i < maxTries-1 {
				glog.V(4).Infof("failed to get instance for machine %s before migrating on try %v with err=%v, will retry", machine.Name, i, err)
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
				glog.V(4).Infof("failed to migrate UID for machine %s  on try %v with err=%v, will retry", machine.Name, i, err)
				continue
			}
			return fmt.Errorf("failed to migrate UID for machine %s: %v", machine.Name, err)
		}
		break
	}
	machine.UID = newUID

	// Step 3: Verify we can successfully get the instance with the new UID
	for i := 0; i < maxTries; i++ {
		if _, err := prov.Get(machine); err != nil {
			if i < maxTries-1 {
				time.Sleep(10 * time.Second)
				glog.V(4).Infof("failed to get instance for machine %s after migrating on try %v with err=%v, will retry", machine.Name, i, err)
				continue
			}
			return fmt.Errorf("failed to get machine %s after migrating UID: %v", machine.Name, err)
		}
		break
	}

	// Step 4: Delete the instance and then verify instance is gone
	for i := 0; i < maxTries; i++ {
		// Deletion part 0: Delete and continue on err if there are tries left
		if err := prov.Delete(machine, machineUpdater); err != nil {
			if i < maxTries-1 {
				glog.V(4).Infof("Failed to delete machine %s on try %v with err=%v, will retry", machine.Name, i, err)
				time.Sleep(10 * time.Second)
				continue
			}
			return fmt.Errorf("failed to delete machine %s: %v", machine.Name, err)
		}

		// Deletion part 1: Get and continue if err != cloudprovidererrors.ErrInstanceNotFound if there are tries left
		_, err = prov.Get(machine)
		if err != nil && err == cloudprovidererrors.ErrInstanceNotFound {
			break
		}
		if i < maxTries-1 {
			glog.V(4).Infof("Get after deleting instance for machine %s did not return ErrInstanceNotFound but err=%v", machine.Name, err)
			// Wait a little, as some providers like AWS delete asynchronously
			time.Sleep(10 * time.Second)
			continue
		}
		return fmt.Errorf("expected ErrInstanceNotFound after deleting instance for machine %s, but got err=%v", machine.Name, err)
	}

	return nil
}

func machineUpdater(machine *v1alpha1.Machine, updater func(*v1alpha1.Machine)) (*v1alpha1.Machine, error) {
	updater(machine)
	return machine, nil
}
