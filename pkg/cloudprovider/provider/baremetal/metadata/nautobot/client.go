/*
Copyright 2021 The Machine Controller Authors.

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

package nautobot

import (
	"fmt"
	"strconv"

	"github.com/kubermatic/machine-controller/pkg/nautobot"
)

type MetadataClientConfig struct {
	Name   string  `json:"name"`
	Config *Config `json:"config"`
}

type Config struct {
	Token     string `json:"token"`
	APIServer string `json:"apiServer"`
	Tag       string `json:"tag"`
}

type Client struct {
	token          string
	apiServer      string
	dcTag          string
	nautobotClient nautobot.Client
}

func NewClient(token, apiServer, dcTag string) (*Client, error) {
	c, err := nautobot.NewDefaultClient(token, dcTag, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to create nautobot metadata client: %v", err)
	}

	return &Client{
		token:          token,
		apiServer:      apiServer,
		dcTag:          dcTag,
		nautobotClient: c,
	}, nil
}

func (c *Client) GetMachineCIDR(deviceID string) (*nautobot.IPInfo, error) {
	activeInterface, err := c.nautobotClient.GetActiveInterface(deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active interface: %v", err)
	}

	params := &nautobot.GetIPParams{
		InterfaceID: activeInterface.ID,
	}

	machineIP, err := c.nautobotClient.GetIP(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine IP: %v", err)
	}

	return machineIP, nil
}

func (c *Client) GetMachineMacAddress(deviceID string) (string, error) {
	activeInterface, err := c.nautobotClient.GetActiveInterface(deviceID)
	if err != nil {
		return "", fmt.Errorf("failed to get active interface: %v", err)
	}

	return activeInterface.MacAddress, nil
}

func (c *Client) GetMachineGatewayIP(ip *nautobot.IPInfo, tag string) (string, error) {
	machinIP, _, maskLength, err := nautobot.CidrToIPAndNetMask(ip.Address)
	if err != nil {
		return "", fmt.Errorf("failed to get prefix ip from CIDR: %v", err)
	}

	prefix, err := c.nautobotClient.GetPrefix(machinIP, ip.Vrf.ID, maskLength)
	if err != nil {
		return "", fmt.Errorf("failed to get prefix: %v", err)
	}

	prefixIP, _, maskLength, err := nautobot.CidrToIPAndNetMask(prefix.Prefix)
	if err != nil {
		return "", fmt.Errorf("failed to get prefix ip: %v", err)
	}

	// this is needed in order to have the mask length slash in the raw query
	formatedPrefixIP := prefixIP + "%2F" + strconv.Itoa(maskLength)
	params := &nautobot.GetIPParams{Parent: formatedPrefixIP, Tag: tag}
	router, err := c.nautobotClient.GetIP(params)

	if err != nil {
		return "", fmt.Errorf("failed to get prefix ip: %v", err)
	}

	gatewayIP, _, _, err := nautobot.CidrToIPAndNetMask(router.Address)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway ip: %v", err)
	}

	return gatewayIP, nil
}

func (c *Client) GetMachineStatus() (string, error) {
	return "", nil
}

func (c *Client) GetActiveDevice() (string, error) {
	device, err := c.nautobotClient.RequestActiveDevice()
	if err != nil {
		return "", fmt.Errorf("failed to get active device: %v", err)
	}

	return device.ID, nil
}

func (c *Client) PatchDeviceStatus(deviceID string, params *nautobot.PatchedDeviceParams) error {
	return c.nautobotClient.PatchDeviceStatus(deviceID, params)
}

func (c *Client) GetDeviceByMachineUID(machineUID string) (*nautobot.DeviceInfo, error) {
	device, err := c.nautobotClient.GetDeviceByAssetTag(machineUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device by machine uid: %v", err)
	}

	return device, nil
}
