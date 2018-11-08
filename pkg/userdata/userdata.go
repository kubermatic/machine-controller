package userdata

import (
	"errors"
	"net"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"
	"github.com/kubermatic/machine-controller/pkg/userdata/coreos"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var (
	ErrProviderNotFound = errors.New("no user data provider for the given os found")

	providers = map[providerconfig.OperatingSystem]Provider{
		providerconfig.OperatingSystemCoreos: convert.NewIgnition(coreos.Provider{}),
		providerconfig.OperatingSystemUbuntu: convert.NewGzip(ubuntu.Provider{}),
		providerconfig.OperatingSystemCentOS: convert.NewGzip(centos.Provider{}),
	}
)

func ForOS(os providerconfig.OperatingSystem) (Provider, error) {
	if p, found := providers[os]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound

}

type Provider interface {
	UserData(spec clusterv1alpha1.MachineSpec, kubeconfig *clientcmdapi.Config, ccProvider cloud.ConfigProvider, clusterDNSIPs []net.IP) (string, error)
}
