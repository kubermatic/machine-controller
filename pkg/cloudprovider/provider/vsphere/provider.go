package vsphere

import (
	"encoding/json"
	"fmt"

	_ "github.com/golang/glog"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	_ "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a VSphere provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloud.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type RawConfig struct {
	TemplateVMName providerconfig.ConfigVarString `json:"templateVMName"`
	Username       providerconfig.ConfigVarString `json:"username"`
	Password       providerconfig.ConfigVarString `json:"password"`
	VSphereURL     providerconfig.ConfigVarString `json:"vsphereURL"`
	Datacenter     providerconfig.ConfigVarString `json:"datacenter"`
	ResourcePool   providerconfig.ConfigVarString `json:"resourcePool"`
	Datastore      providerconfig.ConfigVarString `json:"datastore"`
}

type Config struct {
	TemplateVMName string
	Username       string
	Password       string
	VSphereURL     string
	Datacenter     string
	ResourcePool   string
	Datastore      string
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, bool, error) {
	return spec, false, nil
}

func (p *provider) getConfig(s runtime.RawExtension) (*Config, *providerconfig.Config, error) {
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}

	rawConfig := RawConfig{}
	err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig)

	c := Config{}
	c.TemplateVMName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TemplateVMName)
	if err != nil {
		return nil, nil, err
	}

	c.Username, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Username)
	if err != nil {
		return nil, nil, err
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Password)
	if err != nil {
		return nil, nil, err
	}

	c.VSphereURL, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VSphereURL)
	if err != nil {
		return nil, nil, err
	}

	c.Datacenter, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datacenter)
	if err != nil {
		return nil, nil, err
	}

	c.ResourcePool, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ResourcePool)
	if err != nil {
		return nil, nil, err
	}

	c.Datastore, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datastore)
	if err != nil {
		return nil, nil, err
	}

	return nil, nil, nil
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	//TODO: Implement this
	return nil
}

func (p *provider) Create(machine *v1alpha1.Machine, userdata string) (instance.Instance, error) {
	_, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	//TODO: Implement this
	//instance, err := createVM()
	//err = createCloudConfigIso()
	//err = uploadCloudConfigIso()
	//err = attachCloudConfigIso()
	//err = powerOn(instance)
	return nil, nil
}

func (p *provider) Delete(machine *v1alpha1.Machine) error {
	//TODO: Implement this
	return nil
}

func (p *provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	//TODO: Implement this
	return nil, nil
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	//TODO: Implement this
	return "", "", nil
}
