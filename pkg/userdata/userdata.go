package userdata

import (
	"errors"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/coreos"
)

var (
	ErrProviderNotFound = errors.New("no user data provider for the given os found")

	providers = map[string]Provider{
		"coreos": coreos.Provider{},
	}
)

func ForOS(os string) (Provider, error) {
	if p, found := providers[os]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound
}

type Provider interface {
	UserData(spec machinesv1alpha1.MachineSpec, kubeconfig string, ccProvider cloud.ConfigProvider) (string, error)
}
