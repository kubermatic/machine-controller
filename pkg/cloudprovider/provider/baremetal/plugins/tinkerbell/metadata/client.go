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

package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type MachineMetadata struct {
	CIDR       string `json:"cidr,omitempty"`
	MACAddress string `json:"mac_address,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	Status     string `json:"status,omitempty"`
}

type Config struct {
	Endpoint   string      `json:"endpoint,omitempty"`
	AuthConfig *AuthConfig `json:"authConfig,omitempty"`
}

type AuthMethod string

const (
	BasicAuth   AuthMethod = "BasicAuth"
	BearerToken AuthMethod = "BearerToken"

	defaultTimeout = 30 * time.Second
)

type AuthConfig struct {
	AuthMethod AuthMethod `json:"authMethod"`
	Username   string     `json:"username"`
	Password   string     `json:"password"`
	Token      string     `json:"token"`
}

type Client interface {
	GetMachineMetadata() (*MachineMetadata, error)
}

type defaultClient struct {
	metadataEndpoint string
	authConfig       *AuthConfig
	client           *http.Client
}

func NewMetadataClient(cfg *Config) (Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("machine metadata endpoint cannot be empty")
	}

	client := http.DefaultClient
	client.Timeout = defaultTimeout

	return &defaultClient{
		metadataEndpoint: cfg.Endpoint,
		authConfig:       cfg.AuthConfig,
		client:           client,
	}, nil
}

func (d *defaultClient) GetMachineMetadata() (*MachineMetadata, error) {
	req, err := http.NewRequest(http.MethodGet, d.metadataEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a get metadata request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	d.getAuthMethod(req)

	res, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute get metadata request: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to execute get metadata request with status code: %v", res.StatusCode)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	mdConfig := &MachineMetadata{}
	if err := json.Unmarshal(data, mdConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata config: %w", err)
	}

	return mdConfig, nil
}

func (d *defaultClient) getAuthMethod(req *http.Request) {
	switch d.authConfig.AuthMethod {
	case BasicAuth:
		req.SetBasicAuth(d.authConfig.Username, d.authConfig.Password)
	case BearerToken:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.authConfig.Token))
	}
}
