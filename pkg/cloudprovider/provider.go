/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudprovider

import (
	"errors"

	cloudprovidercache "github.com/kubermatic/machine-controller/pkg/cloudprovider/cache"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/alibaba"
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
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

var (
	cache = cloudprovidercache.New()

	// ErrProviderNotFound tells that the requested cloud provider was not found
	ErrProviderNotFound = errors.New("cloudprovider not found")

	providers = map[providerconfigtypes.CloudProvider]func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider{
		providerconfigtypes.CloudProviderDigitalocean: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return digitalocean.New(cvr)
		},
		providerconfigtypes.CloudProviderAWS: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return aws.New(cvr)
		},
		providerconfigtypes.CloudProviderOpenstack: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return openstack.New(cvr)
		},
		providerconfigtypes.CloudProviderGoogle: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return gce.New(cvr)
		},
		providerconfigtypes.CloudProviderHetzner: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return hetzner.New(cvr)
		},
		providerconfigtypes.CloudProviderLinode: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return linode.New(cvr)
		},
		providerconfigtypes.CloudProviderVsphere: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return vsphere.New(cvr)
		},
		providerconfigtypes.CloudProviderAzure: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return azure.New(cvr)
		},
		providerconfigtypes.CloudProviderPacket: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return packet.New(cvr)
		},
		providerconfigtypes.CloudProviderFake: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return fake.New(cvr)
		},
		providerconfigtypes.CloudProviderKubeVirt: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return kubevirt.New(cvr)
		},
		providerconfig.CloudProviderAlibaba: func(cvr *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
			return alibaba.New(cvr)
		},
	}
)

// ForProvider returns a CloudProvider actuator for the requested provider
func ForProvider(p providerconfigtypes.CloudProvider, cvr *providerconfig.ConfigVarResolver) (cloudprovidertypes.Provider, error) {
	if p, found := providers[p]; found {
		return NewValidationCacheWrappingCloudProvider(p(cvr)), nil
	}
	return nil, ErrProviderNotFound
}
