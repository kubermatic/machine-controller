package cloudprovider

import (
	"crypto/rsa"
	"errors"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/digitalocean"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
)

var (
	ErrProviderNotFound = errors.New("cloudprovider not found")

	providers = map[string]CloudProvider{
		"digitalocean": digitalocean.New(),
		"aws":          aws.New(),
	}
)

func ForProvider(p string) (CloudProvider, error) {
	if p, found := providers[p]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound
}

type CloudProvider interface {
	Validate(machinespec v1alpha1.MachineSpec) error
	Get(machine *v1alpha1.Machine) (instance.Instance, error)
	GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error)
	Create(machine *v1alpha1.Machine, userdata string, key rsa.PublicKey) (instance.Instance, error)
	Delete(machine *v1alpha1.Machine) error
}
