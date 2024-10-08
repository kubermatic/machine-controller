package kubeovn

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

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"k8c.io/machine-controller/pkg/cloudprovider/provider/kubevirt/providernetworks"
	"k8s.io/apimachinery/pkg/types"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type kubeOVNProviderNetwork struct {
	client ctrlruntimeclient.Client
}

func New(client ctrlruntimeclient.Client) (providernetworks.ProviderNetwork, error) {
	return &kubeOVNProviderNetwork{client: client}, nil
}

func (k *kubeOVNProviderNetwork) GetVPC(ctx context.Context, vpcName string) (*providernetworks.VPC, error) {
	vpc := &kubeovnv1.Vpc{}
	if err := k.client.Get(ctx, types.NamespacedName{Name: vpcName}, vpc); err != nil {
		return nil, fmt.Errorf("failed to get VPC %s: %w", vpcName, zap.Error(err))
	}

	return &providernetworks.VPC{
		Name: vpc.Name,
	}, nil
}

func (k *kubeOVNProviderNetwork) GetVPCSubnet(ctx context.Context, subnetName string) (*providernetworks.Subnet, error) {
	vpcSubnet := &kubeovnv1.Subnet{}
	if err := k.client.Get(ctx, types.NamespacedName{Name: subnetName}, vpcSubnet); err != nil {
		return nil, fmt.Errorf("failed to get VPC subnet %s: %w", subnetName, err)
	}

	return &providernetworks.Subnet{
		Name:      vpcSubnet.Name,
		CIDRBlock: vpcSubnet.Spec.CIDRBlock,
	}, nil
}
