package cloudprovider

import (
	"errors"

	cloudprovidercache "github.com/kubermatic/machine-controller/pkg/cloudprovider/cache"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/azure"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/digitalocean"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/fake"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/gce"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/hetzner"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/kubevirt"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/linode"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/packet"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

var (
	cache = cloudprovidercache.New()

	// ErrProviderNotFound tells that the requested cloud provider was not found
	ErrProviderNotFound = errors.New("cloudprovider not found")

	providers = map[providerconfig.CloudProvider]func(cvr *providerconfig.ConfigVarResolver) cloud.Provider{
		providerconfig.CloudProviderDigitalocean: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return digitalocean.New(cvr)
		},
		providerconfig.CloudProviderAWS: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return aws.New(cvr)
		},
		providerconfig.CloudProviderOpenstack: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return openstack.New(cvr)
		},
		providerconfig.CloudProviderGoogle: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return gce.New(cvr)
		},
		providerconfig.CloudProviderHetzner: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return hetzner.New(cvr)
		},
		providerconfig.CloudProviderLinode: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return linode.New(cvr)
		},
		providerconfig.CloudProviderVsphere: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return vsphere.New(cvr)
		},
		providerconfig.CloudProviderAzure: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return azure.New(cvr)
		},
		providerconfig.CloudProviderPacket: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return packet.New(cvr)
		},
		providerconfig.CloudProviderFake: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return fake.New(cvr)
		},
		providerconfig.CloudProviderKubeVirt: func(cvr *providerconfig.ConfigVarResolver) cloud.Provider {
			return kubevirt.New(cvr)
		},
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfig.CloudProvider, cvr *providerconfig.ConfigVarResolver) (cloud.Provider, error) {
	if p, found := providers[p]; found {
		return NewValidationCacheWrappingCloudProvider(p(cvr)), nil
	}
	return nil, ErrProviderNotFound
}
