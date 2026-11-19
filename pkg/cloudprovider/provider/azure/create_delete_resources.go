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
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v9"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v10"
	"go.uber.org/zap"

	"k8c.io/machine-controller/sdk/net"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// hasMachineUIDTag returns true if the given resource tags contain the machine UID tag with the given UID.
func hasMachineUIDTag(tags map[string]*string, machineUID types.UID) bool {
	v, ok := tags[machineUIDTag]
	return ok && v != nil && *v == string(machineUID)
}

// toStringPtrSlice converts a slice of strings into a slice of string pointers,
// as used by the Azure SDK. Returns nil for an empty input so that the field is
// omitted from the request.
func toStringPtrSlice(s []string) []*string {
	if len(s) == 0 {
		return nil
	}

	result := make([]*string, 0, len(s))
	for _, v := range s {
		result = append(result, ptr.To(v))
	}

	return result
}

// isNotFound returns true if the given error is an Azure API error with HTTP status 404.
func isNotFound(err error) bool {
	var respErr *azcore.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound
}

// deleteResource deletes a single resource using the given BeginDelete function and waits for the
// deletion to finish. A resource that no longer exists is treated as successfully deleted.
func deleteResource[T, O any](
	ctx context.Context,
	beginDelete func(context.Context, string, string, *O) (*runtime.Poller[T], error),
	resourceGroup, kind, name string,
) error {
	poller, err := beginDelete(ctx, resourceGroup, name, nil)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}

	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to delete %s %q: %w", kind, name, err)
	}

	return nil
}

// deleteResourcesInParallel deletes all given resources concurrently using the given BeginDelete function
// and waits for all deletions to finish. Errors are collected and returned joined together.
func deleteResourcesInParallel[T, O any](
	ctx context.Context,
	beginDelete func(context.Context, string, string, *O) (*runtime.Poller[T], error),
	resourceGroup, kind string,
	names []string,
) error {
	var (
		wg     sync.WaitGroup
		errsMu sync.Mutex
		errs   []error
	)

	for _, name := range names {
		wg.Go(func() {
			if err := deleteResource(ctx, beginDelete, resourceGroup, kind, name); err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			}
		})
	}

	wg.Wait()

	return errors.Join(errs...)
}

// deleteInterfacesByMachineUID will remove all network interfaces tagged with the specific machine's UID.
// The machine has to be deleted or disassociated with the interfaces beforehand, since Azure won't allow
// us to remove interfaces connected to a VM.
func deleteInterfacesByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	ifClient, err := getInterfacesClient(c)
	if err != nil {
		return fmt.Errorf("failed to create interfaces client: %w", err)
	}

	var ifacesToDelete []string

	for pager := ifClient.NewListPager(c.ResourceGroup, nil); pager.More(); {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list interfaces in resource group %q: %w", c.ResourceGroup, err)
		}

		for _, iface := range page.Value {
			if iface != nil && iface.Name != nil && hasMachineUIDTag(iface.Tags, machineUID) {
				ifacesToDelete = append(ifacesToDelete, *iface.Name)
			}
		}
	}

	return deleteResourcesInParallel(ctx, ifClient.BeginDelete, c.ResourceGroup, "network interface", ifacesToDelete)
}

// deleteIPAddressesByMachineUID will remove public IP addresses tagged with the specific machine's UID.
// Their respective network interfaces have to be deleted or disassociated with the IPs beforehand, since
// Azure won't allow us to remove IPs connected to NICs.
func deleteIPAddressesByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	ipClient, err := getIPClient(c)
	if err != nil {
		return fmt.Errorf("failed to create IP addresses client: %w", err)
	}

	var ipsToDelete []string

	for pager := ipClient.NewListPager(c.ResourceGroup, nil); pager.More(); {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list public IP addresses in resource group %q: %w", c.ResourceGroup, err)
		}

		for _, ip := range page.Value {
			if ip != nil && ip.Name != nil && hasMachineUIDTag(ip.Tags, machineUID) {
				ipsToDelete = append(ipsToDelete, *ip.Name)
			}
		}
	}

	return deleteResourcesInParallel(ctx, ipClient.BeginDelete, c.ResourceGroup, "public IP address", ipsToDelete)
}

func deleteVMsByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	vmClient, err := getVMClient(c)
	if err != nil {
		return err
	}

	var vmsToDelete []string

	for pager := vmClient.NewListPager(c.ResourceGroup, nil); pager.More(); {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list virtual machines in resource group %q: %w", c.ResourceGroup, err)
		}

		for _, vm := range page.Value {
			if vm != nil && vm.Name != nil && hasMachineUIDTag(vm.Tags, machineUID) {
				vmsToDelete = append(vmsToDelete, *vm.Name)
			}
		}
	}

	return deleteResourcesInParallel(ctx, vmClient.BeginDelete, c.ResourceGroup, "VM", vmsToDelete)
}

func deleteDisksByMachineUID(ctx context.Context, c *config, machineUID types.UID) error {
	disksClient, err := getDisksClient(c)
	if err != nil {
		return fmt.Errorf("failed to get disks client: %w", err)
	}

	matchingDisks, err := getDisksByMachineUID(ctx, disksClient, machineUID)
	if err != nil {
		return err
	}

	disksToDelete := make([]string, 0, len(matchingDisks))
	for _, disk := range matchingDisks {
		disksToDelete = append(disksToDelete, *disk.Name)
	}

	return deleteResourcesInParallel(ctx, disksClient.BeginDelete, c.ResourceGroup, "disk", disksToDelete)
}

func getDisksByMachineUID(ctx context.Context, disksClient *armcompute.DisksClient, uid types.UID) ([]*armcompute.Disk, error) {
	var matchingDisks []*armcompute.Disk

	pager := disksClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list disks: %w", err)
		}

		for _, disk := range page.Value {
			if disk != nil && disk.Name != nil && hasMachineUIDTag(disk.Tags, uid) {
				matchingDisks = append(matchingDisks, disk)
			}
		}
	}

	return matchingDisks, nil
}

func createOrUpdatePublicIPAddress(ctx context.Context, log *zap.SugaredLogger, ipName string, ipVersion armnetwork.IPVersion, sku armnetwork.PublicIPAddressSKUName, ipAllocationMethod armnetwork.IPAllocationMethod, machineUID types.UID, c *config) (*armnetwork.PublicIPAddress, error) {
	log.Infow("Creating public IP", "name", ipName)
	ipClient, err := getIPClient(c)
	if err != nil {
		return nil, err
	}

	ipParams := armnetwork.PublicIPAddress{
		Name:     ptr.To(ipName),
		Location: ptr.To(c.Location),
		Properties: &armnetwork.PublicIPAddressPropertiesFormat{
			PublicIPAddressVersion:   ptr.To(ipVersion),
			PublicIPAllocationMethod: ptr.To(ipAllocationMethod),
		},
		Tags:  map[string]*string{machineUIDTag: ptr.To(string(machineUID))},
		Zones: toStringPtrSlice(c.Zones),
		SKU: &armnetwork.PublicIPAddressSKU{
			Name: ptr.To(sku),
		},
	}

	poller, err := ipClient.BeginCreateOrUpdate(ctx, c.ResourceGroup, ipName, ipParams, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create public IP address: %w", err)
	}

	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to retrieve public IP address creation result: %w", err)
	}

	log.Infow("Fetching info for IP address", "name", ipName)
	ip, err := getPublicIPAddress(ctx, ipName, c.ResourceGroup, ipClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info about public IP %q: %w", ipName, err)
	}

	return ip, nil
}

func getPublicIPAddress(ctx context.Context, ipName string, resourceGroup string, ipClient *armnetwork.PublicIPAddressesClient) (*armnetwork.PublicIPAddress, error) {
	resp, err := ipClient.Get(ctx, resourceGroup, ipName, nil)
	if err != nil {
		return nil, err
	}

	return &resp.PublicIPAddress, nil
}

func getSubnet(ctx context.Context, c *config) (*armnetwork.Subnet, error) {
	subnetsClient, err := getSubnetsClient(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnets client: %w", err)
	}

	resp, err := subnetsClient.Get(ctx, c.VNetResourceGroup, c.VNetName, c.SubnetName, nil)
	if err != nil {
		return nil, err
	}

	return &resp.Subnet, nil
}

func getSKU(ctx context.Context, log *zap.SugaredLogger, c *config) (armcompute.ResourceSKU, error) {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	cacheKey := fmt.Sprintf("%s-%s", c.Location, c.VMSize)
	cacheSku, found := cache.Get(cacheKey)
	if found {
		log.Debugw("Found SKU in cache", "key", cacheKey, "sku", cacheSku)
		return cacheSku.(armcompute.ResourceSKU), nil
	}

	skuClient, err := getSKUClient(c)
	if err != nil {
		return armcompute.ResourceSKU{}, fmt.Errorf("failed to (create) SKU client: %w", err)
	}

	pager := skuClient.NewListPager(&armcompute.ResourceSKUsClientListOptions{
		Filter:                   ptr.To(fmt.Sprintf("location eq '%s'", c.Location)),
		IncludeExtendedLocations: ptr.To("false"),
	})

	var sku *armcompute.ResourceSKU

	// only fetch the next page if we haven't found our SKU yet
	for sku == nil && pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return armcompute.ResourceSKU{}, fmt.Errorf("failed to list available SKUs: %w", err)
		}

		for _, skuResult := range page.Value {
			// skip invalid SKU results so we don't trigger a nil pointer exception
			if skuResult == nil || skuResult.ResourceType == nil || skuResult.Name == nil {
				continue
			}

			if *skuResult.ResourceType == "virtualMachines" && *skuResult.Name == c.VMSize {
				sku = skuResult
				break
			}
		}
	}

	if sku == nil {
		return armcompute.ResourceSKU{}, fmt.Errorf("no VM SKU '%s' found for subscription '%s'", c.VMSize, c.SubscriptionID)
	}

	cache.SetDefault(cacheKey, *sku)

	return *sku, nil
}

func getVirtualNetwork(ctx context.Context, c *config) (*armnetwork.VirtualNetwork, error) {
	virtualNetworksClient, err := getVirtualNetworksClient(c)
	if err != nil {
		return nil, err
	}

	resp, err := virtualNetworksClient.Get(ctx, c.VNetResourceGroup, c.VNetName, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualNetwork, nil
}

func createOrUpdateNetworkInterface(ctx context.Context, log *zap.SugaredLogger, ifName string, machineUID types.UID, config *config, publicIP, publicIPv6 *armnetwork.PublicIPAddress, ipFamily net.IPFamily, enableAcceleratedNetworking *bool) (*armnetwork.Interface, error) {
	ifClient, err := getInterfacesClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create interfaces client: %w", err)
	}

	subnet, err := getSubnet(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subnet: %w", err)
	}

	ifSpec := armnetwork.Interface{
		Name:     ptr.To(ifName),
		Location: ptr.To(config.Location),
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: ptr.To("ip-config-1"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						Subnet:                    subnet,
						PrivateIPAllocationMethod: ptr.To(armnetwork.IPAllocationMethodDynamic),
						PublicIPAddress:           publicIP,
						Primary:                   ptr.To(true),
					},
				},
			},
			EnableAcceleratedNetworking: enableAcceleratedNetworking,
		},
		Tags: map[string]*string{machineUIDTag: ptr.To(string(machineUID))},
	}

	if ipFamily.IsDualstack() {
		ifSpec.Properties.IPConfigurations = append(ifSpec.Properties.IPConfigurations, &armnetwork.InterfaceIPConfiguration{
			Name: ptr.To("ip-config-2"),
			Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
				PrivateIPAllocationMethod: ptr.To(armnetwork.IPAllocationMethodDynamic),
				Subnet:                    subnet,
				PublicIPAddress:           publicIPv6,
				Primary:                   ptr.To(false),
				PrivateIPAddressVersion:   ptr.To(armnetwork.IPVersionIPv6),
			},
		})
	}

	if config.SecurityGroupName != "" {
		secGroupClient, err := getSecurityGroupsClient(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create security groups client: %w", err)
		}

		secGroup, err := secGroupClient.Get(ctx, config.ResourceGroup, config.SecurityGroupName, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get securityGroup %q: %w", config.SecurityGroupName, err)
		}
		ifSpec.Properties.NetworkSecurityGroup = &secGroup.SecurityGroup
	}

	log.Infow("Creating/Updating public network interface", "interface", ifName)
	poller, err := ifClient.BeginCreateOrUpdate(ctx, config.ResourceGroup, ifName, ifSpec, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to get interface creation result: %w", err)
	}

	log.Infow("Fetching info about network interface", "interface", ifName)
	iface, err := ifClient.Get(ctx, config.ResourceGroup, ifName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info about interface %q: %w", ifName, err)
	}

	return &iface.Interface, nil
}
