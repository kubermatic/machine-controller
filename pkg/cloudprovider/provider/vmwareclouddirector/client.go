/*
Copyright 2022 The Machine Controller Authors.

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

package vmwareclouddirector

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
)

type Client struct {
	Auth      *Auth
	VCDClient *govcd.VCDClient
}

func NewClient(username, password, org, url, vdc string, allowInsecure bool) (*Client, error) {
	client := Client{
		Auth: &Auth{
			Username:      username,
			Password:      password,
			Organization:  org,
			URL:           url,
			VDC:           vdc,
			AllowInsecure: allowInsecure,
		},
	}

	vcdClient, err := client.GetAuthenticatedClient()
	if err != nil {
		return nil, err
	}

	client.VCDClient = vcdClient
	return &client, nil
}

func (c *Client) GetAuthenticatedClient() (*govcd.VCDClient, error) {
	// Ensure that all required fields for authentication are provided
	// Fail early, without any API calls, if some required field is missing.
	if c.Auth == nil {
		return nil, fmt.Errorf("authentication configuration not provided")
	}
	if c.Auth.Username == "" {
		return nil, fmt.Errorf("username not provided")
	}
	if c.Auth.Password == "" {
		return nil, fmt.Errorf("password not provided")
	}
	if c.Auth.URL == "" {
		return nil, fmt.Errorf("URL not provided")
	}
	if c.Auth.Organization == "" {
		return nil, fmt.Errorf("organization name not provided")
	}

	// Ensure that `/api` suffix exists in the cloud director URL.
	apiEndpoint, err := url.Parse(c.Auth.URL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url '%s': %w", c.Auth.URL, err)
	}
	if !strings.HasSuffix(c.Auth.URL, "/api") {
		apiEndpoint.Path = path.Join(apiEndpoint.Path, "api")
	}

	vcdClient := govcd.NewVCDClient(*apiEndpoint, c.Auth.AllowInsecure)

	err = vcdClient.Authenticate(c.Auth.Username, c.Auth.Password, c.Auth.Organization)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with VMware Cloud Director: %w", err)
	}

	return vcdClient, nil
}

func (c *Client) GetOrganization() (*govcd.Org, error) {
	if c.Auth.Organization == "" {
		return nil, errors.New("organization must be configured")
	}

	org, err := c.VCDClient.GetOrgByNameOrId(c.Auth.Organization)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization '%s': %w", c.Auth.Organization, err)
	}
	return org, err
}

func (c *Client) GetVDCForOrg(org govcd.Org) (*govcd.Vdc, error) {
	if c.Auth.VDC == "" {
		return nil, errors.New("Organization VDC must be configured")
	}
	vcd, err := org.GetVDCByNameOrId(c.Auth.VDC, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get Organization VDC '%s': %w", c.Auth.VDC, err)
	}
	return vcd, err
}

func (c *Client) GetVMByName(vappName, vmName string) (*govcd.VM, error) {
	_, _, vapp, err := c.GetOrganizationVDCAndVapp(vappName)
	if err != nil {
		return nil, err
	}

	// We don't need ID here since we explicitly set the name field when creating the resource.
	vm, err := vapp.GetVMByName(vmName, true)
	if err != nil && errors.Is(err, govcd.ErrorEntityNotFound) {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}
	return vm, err
}

func (c *Client) GetOrganizationVDCAndVapp(vappName string) (*govcd.Org, *govcd.Vdc, *govcd.VApp, error) {
	org, err := c.GetOrganization()
	if err != nil {
		return nil, nil, nil, err
	}

	vdc, err := c.GetVDCForOrg(*org)
	if err != nil {
		return nil, nil, nil, err
	}

	// Ensure that the vApp has already been created.
	vapp, err := vdc.GetVAppByNameOrId(vappName, true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get vApp '%s': %w", vappName, err)
	}
	return org, vdc, vapp, nil
}

// GetVappNetworkType checks if the network exists and returns the network type.
func GetVappNetworkType(networkName string, vapp govcd.VApp) (NetworkType, error) {
	networkConfig, err := vapp.GetNetworkConfig()
	if err != nil {
		return NoneNetworkType, fmt.Errorf("error getting vApp networks: %w", err)
	}

	for _, netConfig := range networkConfig.NetworkConfig {
		if netConfig.NetworkName == networkName || netConfig.ID == networkName {
			switch {
			case netConfig.NetworkName == types.NoneNetwork:
				return NoneNetworkType, nil
			case govcd.IsVappNetwork(netConfig.Configuration):
				return VAppNetworkType, nil
			default:
				return OrgNetworkType, nil
			}
		}
	}
	return NoneNetworkType, fmt.Errorf("network '%s' not found: %w", networkName, err)
}
