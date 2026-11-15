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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v9"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v10"
)

func getCredential(c *config) (azcore.TokenCredential, error) {
	cred, err := azidentity.NewClientSecretCredential(c.TenantID, c.ClientID, c.ClientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client secret credential: %w", err)
	}

	return cred, nil
}

// newClient creates a new Azure SDK client of type T using the
// credentials and subscription found in the given config.
func newClient[T any](c *config, constructor func(string, azcore.TokenCredential, *arm.ClientOptions) (*T, error)) (*T, error) {
	cred, err := getCredential(c)
	if err != nil {
		return nil, err
	}

	return constructor(c.SubscriptionID, cred, nil)
}

func getIPClient(c *config) (*armnetwork.PublicIPAddressesClient, error) {
	return newClient(c, armnetwork.NewPublicIPAddressesClient)
}

func getIPConfigClient(c *config) (*armnetwork.InterfaceIPConfigurationsClient, error) {
	return newClient(c, armnetwork.NewInterfaceIPConfigurationsClient)
}

func getSubnetsClient(c *config) (*armnetwork.SubnetsClient, error) {
	return newClient(c, armnetwork.NewSubnetsClient)
}

func getVirtualNetworksClient(c *config) (*armnetwork.VirtualNetworksClient, error) {
	return newClient(c, armnetwork.NewVirtualNetworksClient)
}

func getSecurityGroupsClient(c *config) (*armnetwork.SecurityGroupsClient, error) {
	return newClient(c, armnetwork.NewSecurityGroupsClient)
}

func getInterfacesClient(c *config) (*armnetwork.InterfacesClient, error) {
	return newClient(c, armnetwork.NewInterfacesClient)
}

func getVMClient(c *config) (*armcompute.VirtualMachinesClient, error) {
	return newClient(c, armcompute.NewVirtualMachinesClient)
}

func getSKUClient(c *config) (*armcompute.ResourceSKUsClient, error) {
	return newClient(c, armcompute.NewResourceSKUsClient)
}

func getDisksClient(c *config) (*armcompute.DisksClient, error) {
	return newClient(c, armcompute.NewDisksClient)
}
