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

package nodemanager

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeManager struct {
	ctx      context.Context
	client   ctrlruntimeclient.Client
	nodeName string
}

func New(ctx context.Context, client ctrlruntimeclient.Client, nodeName string) *NodeManager {
	return &NodeManager{
		ctx:      ctx,
		client:   client,
		nodeName: nodeName,
	}
}

func (nm *NodeManager) GetNode() (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := nm.client.Get(nm.ctx, types.NamespacedName{Name: nm.nodeName}, node); err != nil {
		return nil, fmt.Errorf("failed to get node from lister: %w", err)
	}
	return node, nil
}

func (nm *NodeManager) CordonNode(node *corev1.Node) error {
	if !node.Spec.Unschedulable {
		_, err := nm.updateNode(func(n *corev1.Node) {
			n.Spec.Unschedulable = true
		})
		if err != nil {
			return err
		}
	}

	// Be paranoid and wait until the change got propagated to the lister
	// This assumes that the delay between our lister and the APIserver
	// is smaller or equal to the delay the schedulers lister has - If
	// that is not the case, there is a small chance the scheduler schedules
	// pods in between, those will then get deleted upon node deletion and
	// not evicted
	return wait.Poll(1*time.Second, 10*time.Second, func() (bool, error) {
		node := &corev1.Node{}
		if err := nm.client.Get(nm.ctx, types.NamespacedName{Name: nm.nodeName}, node); err != nil {
			return false, err
		}
		if node.Spec.Unschedulable {
			return true, nil
		}
		return false, nil
	})
}

func (nm *NodeManager) updateNode(modify func(*corev1.Node)) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := nm.client.Get(nm.ctx, types.NamespacedName{Name: nm.nodeName}, node); err != nil {
			return err
		}
		// Apply modifications
		modify(node)
		// Update the node
		return nm.client.Update(nm.ctx, node)
	})

	return node, err
}
