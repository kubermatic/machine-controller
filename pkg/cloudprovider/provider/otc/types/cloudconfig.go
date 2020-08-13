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

package types

import (
	"bytes"
	"encoding/json"
)

type LoadBalancerOpts struct {
	Apiserver        string `json:"apiserver"`
	SecretName       string `json:"secretName"`
	SignerType       string `json:"signerType"`
	ELBAlgorithm     string `json:"elbAlgorithm"`
	TenantID         string `json:"tenantId"`
	Region           string `json:"region"`
	VPCID            string `json:"vpcId"`
	SubnetID         string `json:"subnetId"`
	ECSEndpoint      string `json:"ecsEndpoint"`
	ELBEndpoint      string `json:"elbEndpoint"`
	ALBEndpoint      string `json:"albEndpoint"`
	VPCEndpoint      string `json:"vpcEndpoint"`
	NATEndpoint      string `json:"natEndpoint"`
	EnterpriseEnable string `json:"enterpriseEnable"`
}

type AuthOpts struct {
	SecretName  string `json:"SecretName"`
	AccessKey   string `json:"AccessKey"`
	SecretKey   string `json:"SecretKey"`
	IAMEndpoint string `json:"IAMEndpoint"`
	DomainID    string `json:"DomainID"`
	ProjectID   string `json:"ProjectID"`
	Region      string `json:"Region"`
	Cloud       string `json:"Cloud"`
}

// CloudConfig is used to read and store information from the cloud configuration file
type CloudConfig struct {
	LoadBalancer LoadBalancerOpts `json:"LoadBalancer"`
	Auth         AuthOpts         `json:"Auth"`
}

func CloudConfigToString(c *CloudConfig) (string, error) {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(c); err != nil {
		return "", err
	}

	return buf.String(), nil
}
