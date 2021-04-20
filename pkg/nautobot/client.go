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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type deviceClient struct {
	token          string
	dcTag          string
	nautobotServer string
	useHTTP        bool
	client         *http.Client
}

func NewDeviceClient(token, dcTag, nautobotServer string) (*deviceClient, error) {
	if token == "" || dcTag == "" || nautobotServer == "" {
		return nil, errors.New("nautobot token, server address or site name cannot be empty")
	}

	client := http.DefaultClient
	client.Timeout = 30 * time.Second
	return &deviceClient{
		client:         client,
		dcTag:          dcTag,
		token:          token,
		nautobotServer: nautobotServer,
	}, nil
}

func (dc *deviceClient) RequestActiveDevice() (*NetworkDevice, error) {
	var scheme = "https"
	if dc.useHTTP {
		scheme = "http"
	}

	deviceUrl := url.URL{
		Host:     dc.nautobotServer,
		Path:     "api/dcim/devices/",
		RawQuery: fmt.Sprintf("tag=%s&status=%s&limit=1&offset=0", dc.dcTag, Active),
		Scheme:   scheme,
	}
	deviceRequest, err := http.NewRequest(http.MethodGet, deviceUrl.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch device request: %v", err)
	}

	deviceRequest.Header.Set("Authorization", fmt.Sprintf("Token %s", dc.token))
	deviceRequest.Header.Set("Content-Type", "application/json")

	res, err := dc.client.Do(deviceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device: %v", err)
	}

	device := &NetworkDevice{}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read device response: %v", err)
	}

	if err := json.Unmarshal(data, device); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device data: %v", err)
	}

	return device, nil
}

func (dc *deviceClient) GetActiveInterface(deviceId string) (*InterfaceInfo, error) {
	var scheme = "https"
	if dc.useHTTP {
		scheme = "http"
	}

	deviceUrl := url.URL{
		Host:     dc.nautobotServer,
		Path:     "api/dcim/interfaces/",
		RawQuery: fmt.Sprintf("mgmt_only=false&device_id=%s", deviceId),
		Scheme:   scheme,
	}
	deviceRequest, err := http.NewRequest(http.MethodGet, deviceUrl.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch interface request: %v", err)
	}

	deviceRequest.Header.Set("Authorization", fmt.Sprintf("Token %s", dc.token))
	deviceRequest.Header.Set("Content-Type", "application/json")

	res, err := dc.client.Do(deviceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device interface: %v", err)
	}

	interfaces := &Interface{}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read device interface response: %v", err)
	}

	if err := json.Unmarshal(data, interfaces); err != nil {
		return nil, fmt.Errorf("failed to unmarshal interface data: %v", err)
	}

	for _, i := range interfaces.Results {
		if i.ConnectedEndpointReachable {
			return &i, nil
		}
	}

	return nil, errors.New("no active reachable interfaces found")
}

func (dc *deviceClient) GetIP(interfaceID string) (*IPInfo, error) {
	var scheme = "https"
	if dc.useHTTP {
		scheme = "http"
	}

	ipURL := url.URL{
		Host:     dc.nautobotServer,
		Path:     "api/ipam/ip-addresses/",
		RawQuery: fmt.Sprintf("interface_id=%s", interfaceID),
		Scheme:   scheme,
	}
	deviceRequest, err := http.NewRequest(http.MethodGet, ipURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch ip request: %v", err)
	}

	deviceRequest.Header.Set("Authorization", fmt.Sprintf("Token %s", dc.token))
	deviceRequest.Header.Set("Content-Type", "application/json")

	res, err := dc.client.Do(deviceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ip: %v", err)
	}

	return extractIPFromBody(res.Body)
}

func (dc *deviceClient) GetPrefix(ipAddress, vrfID string, maskLength int) (*PrefixInfo, error) {
	var scheme = "https"
	if dc.useHTTP {
		scheme = "http"
	}

	ipURL := url.URL{
		Host:     dc.nautobotServer,
		Path:     "api/ipam/prefixes/",
		RawQuery: fmt.Sprintf("contains=%s&mask_length=%v&vrf_id=%s", ipAddress, maskLength, vrfID),
		Scheme:   scheme,
	}
	deviceRequest, err := http.NewRequest(http.MethodGet, ipURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch prefix ip request: %v", err)
	}

	deviceRequest.Header.Set("Authorization", fmt.Sprintf("Token %s", dc.token))
	deviceRequest.Header.Set("Content-Type", "application/json")

	res, err := dc.client.Do(deviceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prefix ip: %v", err)
	}

	prefix := &Prefix{}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ip response: %v", err)
	}

	if err := json.Unmarshal(data, prefix); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ip data: %v", err)
	}

	for _, i := range prefix.Results {
		return &i, nil
	}

	return nil, errors.New("no prefix found")
}

func (dc *deviceClient) GetGatewayIP(parent, tag string) (*IPInfo, error) {
	var scheme = "https"
	if dc.useHTTP {
		scheme = "http"
	}

	ipURL := url.URL{
		Host:     dc.nautobotServer,
		Path:     "api/ipam/ip-addresses/",
		RawQuery: "parent=" + parent + "%2F30&tag=" + tag,
		Scheme:   scheme,
	}
	deviceRequest, err := http.NewRequest(http.MethodGet, ipURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch gateway ip request: %v", err)
	}

	deviceRequest.Header.Set("Authorization", fmt.Sprintf("Token %s", dc.token))
	deviceRequest.Header.Set("Content-Type", "application/json")

	res, err := dc.client.Do(deviceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gateway ip: %v", err)
	}

	return extractIPFromBody(res.Body)
}
