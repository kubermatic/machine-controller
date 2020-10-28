package anexia

import (
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"

	"github.com/anexia-it/go-anxcloud/pkg/vsphere/info"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	v1 "k8s.io/api/core/v1"
)

type anexiaInstance struct {
	info *info.Info
}

func (ai *anexiaInstance) Name() string {
	if ai.info == nil {
		return ""
	}

	return ai.info.Name
}

func (ai *anexiaInstance) ID() string {
	if ai.info == nil {
		return ""
	}

	return ai.info.Identifier
}

func (ai *anexiaInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}

	if ai.info == nil {
		return addresses
	}

	for _, network := range ai.info.Network {
		for _, ip := range network.IPv4 {
			addresses[ip] = v1.NodeExternalIP
		}
		for _, ip := range network.IPv6 {
			addresses[ip] = v1.NodeExternalIP
		}

		// TODO mark RFC1918 and RFC4193 addresses as internal
	}

	return addresses
}

func (ai *anexiaInstance) Status() instance.Status {
	if ai.info != nil {
		if ai.info.Status == anxtypes.MachinePoweredOn {
			return instance.StatusRunning
		}
	}
	return instance.StatusUnknown
}
