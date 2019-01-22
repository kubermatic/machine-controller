package fake

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type provider struct{}

type CloudProviderSpec struct {
	PassValidation bool `json:"passValidation"`
}

type CloudProviderInstance struct{}

func (f CloudProviderInstance) Name() string {
	return ""
}
func (f CloudProviderInstance) ID() string {
	return ""
}
func (f CloudProviderInstance) Addresses() []string {
	return nil
}
func (f CloudProviderInstance) Status() instance.Status {
	return instance.StatusUnknown
}

// New returns a fake cloud provider
func New(_ *providerconfig.ConfigVarResolver) cloud.Provider {
	return &provider{}
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its FakeCloudProviderSpec
func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(machinespec.ProviderSpec.Value.Raw, &pconfig)
	if err != nil {
		return err
	}

	fakeCloudProviderSpec := CloudProviderSpec{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &fakeCloudProviderSpec); err != nil {
		return err
	}

	if fakeCloudProviderSpec.PassValidation {
		glog.V(4).Infof("succeeding validation as requested")
		return nil
	}

	glog.V(4).Infof("failing validation as requested")
	return fmt.Errorf("failing validation as requested")
}

func (p *provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	return CloudProviderInstance{}, nil
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

// Create creates a cloud instance according to the given machine
func (p *provider) Create(_ *v1alpha1.Machine, _ *cloud.MachineCreateDeleteData, _ string) (instance.Instance, error) {
	return CloudProviderInstance{}, nil
}

func (p *provider) Cleanup(_ *v1alpha1.Machine, _ *cloud.MachineCreateDeleteData) (bool, error) {
	return true, nil
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(_ v1alpha1.MachineList) error {
	return fmt.Errorf("Not implemented")
}
