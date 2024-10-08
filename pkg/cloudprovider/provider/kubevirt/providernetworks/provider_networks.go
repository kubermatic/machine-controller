/*
 Copyright 2024 The Kubermatic Kubernetes Platform contributors.
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

package providernetworks

import (
	"context"
)

type SupportedProviderNetworks string

const (
	KubeOVN SupportedProviderNetworks = "kubeovn"
)

// ProviderNetwork describes the infra cluster network fabric that is being used. These fabrics could be a as simple cni
// specific features up to full-blown networking components such as VPCs and Subnets.
type ProviderNetwork interface {
	GetVPC(ctx context.Context, vpcName string) (*VPC, error)
	GetVPCSubnet(ctx context.Context, subnetName string) (*Subnet, error)
}

// VPC  is a virtual network dedicated to a single tenant within a KubeVirt, where the resources in the VPC
// is isolated from any other resources within the KubeVirt infra cluster.
type VPC struct {
	Name string `json:"name"`
}

// Subnet a smaller, segmented portion of a larger network, like a Virtual Private Cloud (VPC).
type Subnet struct {
	Name       string   `json:"name"`
	CIDRBlock  string   `json:"cidrBlock"`
	GatewayIP  string   `json:"gatewayIP,omitempty"`
	ExcludeIPs []string `json:"excludeIP,omitempty"`
}
