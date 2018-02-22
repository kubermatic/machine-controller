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

	providers = map[providerconfig.CloudProvider]func(key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) cloud.Provider{
		providerconfig.CloudProviderDigitalocean: func(key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return digitalocean.New(key, cvr)
		},
		providerconfig.CloudProviderAWS: func(key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return aws.New(key, cvr)
		},
		providerconfig.CloudProviderOpenstack: func(key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return openstack.New(key, cvr)
		},
		providerconfig.CloudProviderHetzner: func(key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return hetzner.New(key, cvr)
		},
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfig.CloudProvider, key *machinessh.PrivateKey, cvr *providerconfig.ConfigVarResolver) (cloud.Provider, error) {
	if p, found := providers[p]; found {
		return p(key, cvr), nil
	}
	return nil, ErrProviderNotFound
}
