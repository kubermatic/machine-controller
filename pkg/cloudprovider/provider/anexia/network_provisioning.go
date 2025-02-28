/*
Copyright 2024 The Machine Controller Authors.

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

package anexia

import (
	"context"
	"sync"
	"time"

	anxclient "go.anx.io/go-anxcloud/pkg/client"
	anxaddr "go.anx.io/go-anxcloud/pkg/ipam/address"
	anxvm "go.anx.io/go-anxcloud/pkg/vsphere/provisioning/vm"
	"go.uber.org/zap"

	"k8c.io/machine-controller/sdk/apis/cluster/common"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
)

func networkInterfacesForProvisioning(ctx context.Context, log *zap.SugaredLogger, client anxclient.Client) ([]anxvm.Network, error) {
	reconcileContext := getReconcileContext(ctx)

	config := reconcileContext.Config
	status := reconcileContext.Status

	// make sure we have the status.Networks array allocated to fill it with
	// data, warning if we already have something but not matching the
	// configuration.
	if len(status.Networks) != len(config.Networks) {
		if len(status.Networks) != 0 {
			log.Warn("size of status.Networks != config.Networks, this should not happen in normal operation - ignoring existing status")
		}

		status.Networks = make([]anxtypes.NetworkStatus, len(config.Networks))
	}

	ret := make([]anxvm.Network, len(config.Networks))
	for netIndex, network := range config.Networks {
		networkStatus := &status.Networks[netIndex]
		addresses := make([]string, len(network.Prefixes))

		for prefixIndex, prefix := range network.Prefixes {
			// make sure we have the address status array allocated to fill it
			// with our IP reserve status, warning if we already have something
			// there but not matching the configuration.
			if len(networkStatus.Addresses) != len(network.Prefixes) {
				if len(networkStatus.Addresses) != 0 {
					log.Warnf("size of status.Networks[%[1]v].Addresses != config.Networks[%[1]v].Prefixes, this should not happen in normal operation - ignoring existing status", netIndex)
				}

				networkStatus.Addresses = make([]anxtypes.NetworkAddressStatus, len(network.Prefixes))
			}

			reservedIP, err := getIPAddress(ctx, log, &network, prefix, &networkStatus.Addresses[prefixIndex], client)
			if err != nil {
				return nil, newError(common.CreateMachineError, "failed to reserve IP: %v", err)
			}

			addresses[prefixIndex] = reservedIP
		}

		ret[netIndex] = anxvm.Network{
			VLAN: network.VlanID,
			IPs:  addresses,

			// the one NIC type supported by the ADC API
			NICType: anxtypes.VmxNet3NIC,
		}
	}

	return ret, nil
}

// ENGSUP-3404 is about a race condition when reserving IPs - two calls for one
// IP each, coming in at "nearly the same millisecond", can result in both
// reserving the same IP.
//
// The proposed fix was to reserve n IPs in one call, but that would require
// lots of architecture changes - we can't really do the "reserve IPs for all
// the Machines we want to create and then create the Machines" here.
//
// This mutex alleviates the issue enough, that we didn't see it in a long
// time. It's not impossible this race condition was fixed in some other change
// and we weren't told, but I'd rather not test this and risk having problems
// again.. it's not too expensive of a Mutex.
var _engsup3404mutex sync.Mutex

func getIPAddress(ctx context.Context, log *zap.SugaredLogger, network *resolvedNetwork, prefix string, status *anxtypes.NetworkAddressStatus, client anxclient.Client) (string, error) {
	reconcileContext := getReconcileContext(ctx)

	// only use IP if it is still unbound
	if status.ReservedIP != "" && status.IPState == anxtypes.IPStateUnbound && (!status.IPProvisioningExpires.IsZero() && status.IPProvisioningExpires.After(time.Now())) {
		log.Infow("Re-using already provisioned IP", "ip", status.ReservedIP)
		return status.ReservedIP, nil
	}

	_engsup3404mutex.Lock()
	defer _engsup3404mutex.Unlock()

	log.Info("Creating a new IP for machine")
	addrAPI := anxaddr.NewAPI(client)
	config := reconcileContext.Config

	res, err := addrAPI.ReserveRandom(ctx, anxaddr.ReserveRandom{
		LocationID:        config.LocationID,
		VlanID:            network.VlanID,
		PrefixID:          prefix,
		ReservationPeriod: uint(anxtypes.IPProvisioningExpires / time.Second),
		Count:             1,
	})
	if err != nil {
		return "", newError(common.InvalidConfigurationMachineError, "failed to reserve an ip address: %v", err)
	}

	if len(res.Data) < 1 {
		return "", newError(common.InsufficientResourcesMachineError, "no ip address is available for this machine")
	}

	ip := res.Data[0].Address
	status.ReservedIP = ip
	status.IPState = anxtypes.IPStateUnbound
	status.IPProvisioningExpires = time.Now().Add(anxtypes.IPProvisioningExpires)

	return ip, nil
}

func networkReservedAddresses(status *anxtypes.ProviderStatus) []string {
	ret := make([]string, 0)
	for _, network := range status.Networks {
		for _, address := range network.Addresses {
			if address.ReservedIP != "" && address.IPState == anxtypes.IPStateBound {
				ret = append(ret, address.ReservedIP)
			}
		}
	}

	return ret
}

func networkStatusMarkIPsBound(status *anxtypes.ProviderStatus) {
	for network := range status.Networks {
		for addr := range status.Networks[network].Addresses {
			status.Networks[network].Addresses[addr].IPState = anxtypes.IPStateBound
		}
	}
}
