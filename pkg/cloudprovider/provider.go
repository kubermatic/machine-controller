package cloudprovider

import (
	"errors"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/digitalocean"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/hetzner"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	machinessh "github.com/kubermatic/machine-controller/pkg/ssh"
)

var (
	// ErrProviderNotFound tells that the requested cloud provider was not found
	ErrProviderNotFound = errors.New("cloudprovider not found")

	providers = map[providerconfig.CloudProvider]func(key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) cloud.Provider{
		providerconfig.CloudProviderDigitalocean: func(key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) cloud.Provider {
			return digitalocean.New(key, skg)
		},
		providerconfig.CloudProviderAWS: func(key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) cloud.Provider {
			return aws.New(key, skg)
		},
		providerconfig.CloudProviderOpenstack: func(key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) cloud.Provider {
			return openstack.New(key, skg)
		},
		providerconfig.CloudProviderHetzner: func(key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) cloud.Provider {
			return hetzner.New(key, skg)
		},
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfig.CloudProvider, key *machinessh.PrivateKey, skg *providerconfig.SecretKeyGetter) (cloud.Provider, error) {
	if p, found := providers[p]; found {
		return p(key, skg), nil
	}
	return nil, ErrProviderNotFound
}
