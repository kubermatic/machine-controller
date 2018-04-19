package userdata

import (
	"errors"
	"net"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/coreos"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	ErrProviderNotFound = errors.New("no user data provider for the given os found")

	providers = map[providerconfig.OperatingSystem]Provider{
		providerconfig.OperatingSystemCoreos: coreos.Provider{},
		providerconfig.OperatingSystemUbuntu: ubuntu.Provider{},
		providerconfig.OperatingSystemCentOS: centos.Provider{},
	}
)

func ForOS(os providerconfig.OperatingSystem) (Provider, error) {
	if p, found := providers[os]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound
}

type Provider interface {
	UserData(spec machinesv1alpha1.MachineSpec, kubeconfig *clientcmdapi.Config, ccProvider cloud.ConfigProvider, clusterDNSIPs []net.IP) (string, error)
	SupportedContainerRuntimes() []machinesv1alpha1.ContainerRuntimeInfo
}
