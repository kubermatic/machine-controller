package userdata

import (
	"errors"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/coreos"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"
)

var (
	ErrProviderNotFound = errors.New("no user data provider for the given os found")

	providers = map[providerconfig.OperatingSystem]Provider{
		providerconfig.OperatingSystemCoreos: coreos.Provider{},
		providerconfig.OperatingSystemUbuntu: ubuntu.Provider{},
	}
)

func ForOS(os providerconfig.OperatingSystem) (Provider, error) {
	if p, found := providers[os]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound
}

type Provider interface {
	UserData(spec machinesv1alpha1.MachineSpec, kubeconfig string, ccProvider cloud.ConfigProvider) (string, error)
}
