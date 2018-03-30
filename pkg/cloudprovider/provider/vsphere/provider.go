package vsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware/govmomi"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
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
	Cluster        providerconfig.ConfigVarString `json:"cluster"`
	Datastore      providerconfig.ConfigVarString `json:"datastore"`
	AllowInsecure  providerconfig.ConfigVarBool   `json:"allowInsecure"`
}

type Config struct {
	TemplateVMName string
	Username       string
	Password       string
	VSphereURL     string
	Datacenter     string
	Cluster        string
	Datastore      string
	AllowInsecure  bool
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, bool, error) {
	return spec, false, nil
}

func getClient(username, password, address string, allowInsecure bool) (*govmomi.Client, error) {
	clientUrl, err := url.Parse(fmt.Sprintf("%s/sdk", address))
	if err != nil {
		return nil, err
	}
	clientUrl.User = url.UserPassword(username, password)

	return govmomi.NewClient(context.TODO(), clientUrl, allowInsecure)
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

	c.Cluster, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Cluster)
	if err != nil {
		return nil, nil, err
	}

	c.Datastore, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datastore)
	if err != nil {
		return nil, nil, err
	}

	c.AllowInsecure, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.AllowInsecure)
	if err != nil {
		return nil, nil, err
	}

	return &c, &pconfig, nil
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	//TODO: Implement this
	return nil
}

func (p *provider) Create(machine *v1alpha1.Machine, userdata string) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	//TODO: Create provider type to not manually pass the client around
	client, err := getClient(config.Username, config.Password, config.VSphereURL, config.AllowInsecure)
	if err != nil {
		return nil, fmt.Errorf("failed to get vsphere client: '%v'", err)
	}

	vmName, err := CreateLinkClonedVm(machine.Spec.Name, config.TemplateVMName, config.Datacenter, config.Cluster, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create linked vm: '%v'", err)
	}

	glog.V(2).Infof("Successfully created a vm with name '%s'", vmName)

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
	_, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	//TODO: Implement this
	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	//TODO: Implement this
	return "", "", nil
}
