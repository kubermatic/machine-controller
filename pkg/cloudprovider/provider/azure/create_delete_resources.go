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

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-11-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-05-01/network"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

// deleteInterfacesByMachineUID will remove all network interfaces tagged with the specific machine's UID.
// The machine has to be deleted or disassociated with the interfaces beforehand, since Azure won't allow
// us to remove interfaces connected to a VM.
func deleteInterfacesByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	ifClient, err := getInterfacesClient(c)
	if err != nil {
		return fmt.Errorf("failed to create interfaces client: %w", err)
	}

	list, err := ifClient.List(ctx, c.ResourceGroup)
	if err != nil {
		return fmt.Errorf("failed to list interfaces in resource group %q", c.ResourceGroup)
	}

	var allInterfaces []network.Interface

	for list.NotDone() {
		allInterfaces = append(allInterfaces, list.Values()...)
		if err = list.NextWithContext(ctx); err != nil {
			return fmt.Errorf("failed to iterate the result list: %w", err)
		}
	}

	for _, iface := range allInterfaces {
		if iface.Tags != nil && iface.Tags[machineUIDTag] != nil && *iface.Tags[machineUIDTag] == string(machineUID) {
			future, err := ifClient.Delete(ctx, c.ResourceGroup, *iface.Name)
			if err != nil {
				return err
			}

			if err = future.WaitForCompletionRef(ctx, ifClient.Client); err != nil {
				return err
			}
		}
	}

	return nil
}

// deleteIPAddressesByMachineUID will remove public IP addresses tagged with the specific machine's UID.
// Their respective network interfaces have to be deleted or disassociated with the IPs beforehand, since
// Azure won't allow us to remove IPs connected to NICs.
func deleteIPAddressesByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	ipClient, err := getIPClient(c)
	if err != nil {
		return fmt.Errorf("failed to create IP addresses client: %w", err)
	}

	list, err := ipClient.List(ctx, c.ResourceGroup)
	if err != nil {
		return fmt.Errorf("failed to list public IP addresses in resource group %q", c.ResourceGroup)
	}

	var allIPs []network.PublicIPAddress

	for list.NotDone() {
		allIPs = append(allIPs, list.Values()...)
		if err = list.Next(); err != nil {
			return fmt.Errorf("failed to iterate the result list: %w", err)
		}
	}

	for _, ip := range allIPs {
		if ip.Tags != nil && ip.Tags[machineUIDTag] != nil && *ip.Tags[machineUIDTag] == string(machineUID) {
			future, err := ipClient.Delete(ctx, c.ResourceGroup, *ip.Name)
			if err != nil {
				return err
			}

			if err = future.WaitForCompletionRef(ctx, ipClient.Client); err != nil {
				return err
			}
		}
	}

	return nil
}

func deleteVMsByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	vmClient, err := getVMClient(c)
	if err != nil {
		return err
	}

	list, err := vmClient.List(ctx, c.ResourceGroup, "")

	if err != nil {
		return err
	}

	var allServers []compute.VirtualMachine

	for list.NotDone() {
		allServers = append(allServers, list.Values()...)
		if err = list.Next(); err != nil {
			return fmt.Errorf("failed to iterate the result list: %w", err)
		}
	}

	for _, vm := range allServers {
		if vm.Tags != nil && vm.Tags[machineUIDTag] != nil && *vm.Tags[machineUIDTag] == string(machineUID) {
			future, err := vmClient.Delete(ctx, c.ResourceGroup, *vm.Name, nil)
			if err != nil {
				return err
			}

			if err = future.WaitForCompletionRef(ctx, vmClient.Client); err != nil {
				return err
			}
		}
	}

	return nil
}

func deleteDisksByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	disksClient, err := getDisksClient(c)
	if err != nil {
		return fmt.Errorf("failed to get disks client: %w", err)
	}

	matchingDisks, err := getDisksByMachineUID(ctx, disksClient, c, machineUID)
	if err != nil {
		return err
	}

	for _, disk := range matchingDisks {
		future, err := disksClient.Delete(ctx, c.ResourceGroup, *disk.Name)
		if err != nil {
			return fmt.Errorf("failed to delete disk %s: %w", *disk.Name, err)
		}

		if err = future.WaitForCompletionRef(ctx, disksClient.Client); err != nil {
			return fmt.Errorf("failed to wait for deletion of disk %s: %w", *disk.Name, err)
		}
	}

	return nil
}

func getDisksByMachineUID(ctx context.Context, disksClient *compute.DisksClient, c *config, UID types.UID) ([]compute.Disk, error) {
	list, err := disksClient.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %w", err)
	}

	var allDisks, matchingDisks []compute.Disk
	for list.NotDone() {
		allDisks = append(allDisks, list.Values()...)
		if err = list.Next(); err != nil {
			return nil, fmt.Errorf("failed to iterate the result list: %w", err)
		}
	}

	for _, disk := range allDisks {
		if disk.Tags != nil && disk.Tags[machineUIDTag] != nil && *disk.Tags[machineUIDTag] == string(UID) {
			matchingDisks = append(matchingDisks, disk)
		}
	}

	return matchingDisks, nil
}

func createOrUpdatePublicIPAddress(ctx context.Context, ipName string, ipVersion network.IPVersion, sku network.PublicIPAddressSkuName, ipAllocationMethod network.IPAllocationMethod, machineUID types.UID, c *config) (*network.PublicIPAddress, error) {
	klog.Infof("Creating public IP %q", ipName)
	ipClient, err := getIPClient(c)
	if err != nil {
		return nil, err
	}

	ipParams := network.PublicIPAddress{
		Name:     to.StringPtr(ipName),
		Location: to.StringPtr(c.Location),
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			PublicIPAddressVersion:   ipVersion,
			PublicIPAllocationMethod: ipAllocationMethod,
		},
		Tags:  map[string]*string{machineUIDTag: to.StringPtr(string(machineUID))},
		Zones: &c.Zones,
		Sku: &network.PublicIPAddressSku{
			Name: sku,
		},
	}

	future, err := ipClient.CreateOrUpdate(ctx, c.ResourceGroup, ipName, ipParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create public IP address: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, ipClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve public IP address creation result: %w", err)
	}

	if _, err = future.Result(*ipClient); err != nil {
		return nil, fmt.Errorf("failed to create public IP address: %w", err)
	}

	klog.Infof("Fetching info for IP address %q", ipName)
	ip, err := getPublicIPAddress(ctx, ipName, c.ResourceGroup, ipClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info about public IP %q: %w", ipName, err)
	}

	return ip, nil
}

func getPublicIPAddress(ctx context.Context, ipName string, resourceGroup string, ipClient *network.PublicIPAddressesClient) (*network.PublicIPAddress, error) {
	ip, err := ipClient.Get(ctx, resourceGroup, ipName, "")
	if err != nil {
		return nil, err
	}

	return &ip, nil
}

func getSubnet(ctx context.Context, c *config) (network.Subnet, error) {
	subnetsClient, err := getSubnetsClient(c)
	if err != nil {
		return network.Subnet{}, fmt.Errorf("failed to create subnets client: %w", err)
	}

	return subnetsClient.Get(ctx, c.VNetResourceGroup, c.VNetName, c.SubnetName, "")
}

func getSKU(ctx context.Context, c *config) (compute.ResourceSku, error) {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	cacheKey := fmt.Sprintf("%s-%s", c.Location, c.VMSize)
	cacheSku, found := cache.Get(cacheKey)
	if found {
		klog.V(3).Info("found SKU in cache!")
		return cacheSku.(compute.ResourceSku), nil
	}

	skuClient, err := getSKUClient(c)
	if err != nil {
		return compute.ResourceSku{}, fmt.Errorf("failed to (create) SKU client: %w", err)
	}

	skuPages, err := skuClient.List(ctx, fmt.Sprintf("location eq '%s'", c.Location), "false")
	if err != nil {
		return compute.ResourceSku{}, fmt.Errorf("failed to list available SKUs: %w", err)
	}

	var sku *compute.ResourceSku

	for skuPages.NotDone() && sku == nil {
		skus := skuPages.Values()
		for i, skuResult := range skus {
			// skip invalid SKU results so we don't trigger a nil pointer exception
			if skuResult.ResourceType == nil || skuResult.Name == nil {
				continue
			}

			if *skuResult.ResourceType == "virtualMachines" && *skuResult.Name == c.VMSize {
				sku = &skus[i]
				break
			}
		}

		// only fetch the next page if we haven't found our SKU yet
		if sku == nil {
			if err := skuPages.NextWithContext(ctx); err != nil {
				return compute.ResourceSku{}, fmt.Errorf("failed to list available SKUs: %w", err)
			}
		}
	}

	if sku == nil {
		return compute.ResourceSku{}, fmt.Errorf("no VM SKU '%s' found for subscription '%s'", c.VMSize, c.SubscriptionID)
	}

	cache.SetDefault(cacheKey, *sku)

	return *sku, nil
}

func getVirtualNetwork(ctx context.Context, c *config) (network.VirtualNetwork, error) {
	virtualNetworksClient, err := getVirtualNetworksClient(c)
	if err != nil {
		return network.VirtualNetwork{}, err
	}

	return virtualNetworksClient.Get(ctx, c.VNetResourceGroup, c.VNetName, "")
}

func createOrUpdateNetworkInterface(ctx context.Context, ifName string, machineUID types.UID, config *config, publicIP, publicIPv6 *network.PublicIPAddress, ipFamily util.IPFamily) (*network.Interface, error) {
	ifClient, err := getInterfacesClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create interfaces client: %w", err)
	}

	subnet, err := getSubnet(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subnet: %w", err)
	}

	ifSpec := network.Interface{
		Name:     to.StringPtr(ifName),
		Location: &config.Location,
		InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
			IPConfigurations: &[]network.InterfaceIPConfiguration{},
		},
		Tags: map[string]*string{machineUIDTag: to.StringPtr(string(machineUID))},
	}

	*ifSpec.InterfacePropertiesFormat.IPConfigurations = append(*ifSpec.InterfacePropertiesFormat.IPConfigurations, network.InterfaceIPConfiguration{
		Name: to.StringPtr("ip-config-1"),
		InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
			Subnet:                    &subnet,
			PrivateIPAllocationMethod: network.IPAllocationMethodDynamic,
			PublicIPAddress:           publicIP,
			Primary:                   to.BoolPtr(true),
		},
	})

	if ipFamily == util.DualStack {
		*ifSpec.InterfacePropertiesFormat.IPConfigurations = append(*ifSpec.InterfacePropertiesFormat.IPConfigurations, network.InterfaceIPConfiguration{
			Name: to.StringPtr("ip-config-2"),
			InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
				PrivateIPAllocationMethod: network.IPAllocationMethodDynamic,
				Subnet:                    &subnet,
				PublicIPAddress:           publicIPv6,
				Primary:                   to.BoolPtr(false),
				PrivateIPAddressVersion:   network.IPVersionIPv6,
			},
		})
	}

	if config.SecurityGroupName != "" {
		authorizer, err := auth.NewClientCredentialsConfig(config.ClientID, config.ClientSecret, config.TenantID).Authorizer()
		if err != nil {
			return nil, fmt.Errorf("failed to create authorizer for security groups: %w", err)
		}
		secGroupClient := network.NewSecurityGroupsClient(config.SubscriptionID)
		secGroupClient.Authorizer = authorizer
		secGroup, err := secGroupClient.Get(ctx, config.ResourceGroup, config.SecurityGroupName, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get securityGroup %q: %w", config.SecurityGroupName, err)
		}
		ifSpec.NetworkSecurityGroup = &secGroup
	}
	klog.Infof("Creating/Updating public network interface %q", ifName)
	future, err := ifClient.CreateOrUpdate(ctx, config.ResourceGroup, ifName, ifSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, ifClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface creation response: %w", err)
	}

	_, err = future.Result(*ifClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface creation result: %w", err)
	}

	klog.Infof("Fetching info about network interface %q", ifName)
	iface, err := ifClient.Get(ctx, config.ResourceGroup, ifName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info about interface %q: %w", ifName, err)
	}

	return &iface, nil
}
