package alibaba

import (
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type RawConfig struct {
}

type Config struct {
}

type alibabaServer struct {
}

func (a *alibabaServer) Name() string {
	panic("implement me")
}

func (a *alibabaServer) ID() string {
	panic("implement me")
}

func (a *alibabaServer) Addresses() []string {
	panic("implement me")
}

func (a *alibabaServer) Status() instance.Status {
	panic("implement me")
}

// New returns a Kubevirt provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	panic("implement me")
}

func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
	panic("implement me")
}

func (p *provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	panic("implement me")
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	panic("implement me")
}

func (p *provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	panic("implement me")
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	panic("implement me")
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	panic("implement me")
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	panic("implement me")
}

func (p *provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	panic("implement me")
}
