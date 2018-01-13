package cloudprovider

import (
	"errors"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/digitalocean"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

var (
	// ErrProviderNotFound tells that the requested cloud provider was not found
	ErrProviderNotFound = errors.New("cloudprovider not found")

	providers = map[providerconfig.CloudProvider]cloud.Provider{
		providerconfig.CloudProviderDigitalocean: digitalocean.New(),
		providerconfig.CloudProviderAWS:          aws.New(),
		providerconfig.CloudProviderOpenstack:    openstack.New(),
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfig.CloudProvider) (cloud.Provider, error) {
	if p, found := providers[p]; found {
		return p, nil
	}
	return nil, ErrProviderNotFound
}
