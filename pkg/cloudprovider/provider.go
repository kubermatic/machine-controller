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

	providers = map[providerconfig.CloudProvider]func(key *machinessh.PrivateKey) cloud.Provider{
		providerconfig.CloudProviderDigitalocean: func(key *machinessh.PrivateKey) cloud.Provider { return digitalocean.New(key) },
		providerconfig.CloudProviderAWS:          func(key *machinessh.PrivateKey) cloud.Provider { return aws.New(key) },
		providerconfig.CloudProviderOpenstack:    func(key *machinessh.PrivateKey) cloud.Provider { return openstack.New(key) },
		providerconfig.CloudProviderHetzner:      func(key *machinessh.PrivateKey) cloud.Provider { return hetzner.New(key) },
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfig.CloudProvider, key *machinessh.PrivateKey) (cloud.Provider, error) {
	if p, found := providers[p]; found {
		return p(key), nil
	}
	return nil, ErrProviderNotFound
}
