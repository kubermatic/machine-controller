/*
Copyright 2023 The Machine Controller Authors.

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

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/smithy-go/ptr"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// createOrUpdateVMAntiAffinityRule creates or updates an anti affinity rule with the name in the given cluster.
// VMs are attached to the rule based on their folder path and name prefix in vsphere.
// A minimum of two VMs is required.
func (p *provider) createOrUpdateVMAntiAffinityRule(ctx context.Context, session *Session, name string, config *Config) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	cluster, err := session.Finder.ClusterComputeResource(ctx, config.Cluster)
	if err != nil {
		return err
	}

	vmsInFolder, err := session.Finder.VirtualMachineList(ctx, strings.Join([]string{config.Folder, "*"}, "/"))
	if err != nil {
		if errors.Is(err, &find.NotFoundError{}) {
			return removeVMAntiAffinityRule(ctx, session, config.Cluster, name)
		}
		return err
	}

	var ruleVMRef []types.ManagedObjectReference
	for _, vm := range vmsInFolder {
		if strings.HasPrefix(vm.Name(), name) {
			ruleVMRef = append(ruleVMRef, vm.Reference())
		}
	}

	// minimum of two vms required
	if len(ruleVMRef) < 2 {
		return removeVMAntiAffinityRule(ctx, session, config.Cluster, name)
	}

	info, err := findClusterAntiAffinityRuleByName(ctx, cluster, name)
	if err != nil {
		return err
	}

	operation := types.ArrayUpdateOperationEdit

	//create new rule
	if info == nil {
		info = &types.ClusterAntiAffinityRuleSpec{
			ClusterRuleInfo: types.ClusterRuleInfo{
				Enabled:     ptr.Bool(true),
				Mandatory:   ptr.Bool(false),
				Name:        name,
				UserCreated: ptr.Bool(true),
			},
		}
		operation = types.ArrayUpdateOperationAdd
	}

	info.Vm = ruleVMRef
	spec := &types.ClusterConfigSpecEx{
		RulesSpec: []types.ClusterRuleSpec{
			{
				ArrayUpdateSpec: types.ArrayUpdateSpec{
					Operation: operation,
				},
				Info: info,
			},
		},
	}

	task, err := cluster.Reconfigure(ctx, spec, true)
	if err != nil {
		return err
	}

	err = task.Wait(ctx)
	if err != nil {
		return err
	}

	return waitForRule(ctx, cluster, info)
}

// waitForRule checks periodically the vsphere api for the ClusterAntiAffinityRule and returns error if the rule was not found after a timeout.
func waitForRule(ctx context.Context, cluster *object.ClusterComputeResource, rule *types.ClusterAntiAffinityRuleSpec) error {
	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer timeout.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:

			info, err := findClusterAntiAffinityRuleByName(ctx, cluster, rule.Name)
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(rule, info) {
				return fmt.Errorf("expected anti affinity changes not found in vsphere")
			}
		case <-ticker.C:
			info, err := findClusterAntiAffinityRuleByName(ctx, cluster, rule.Name)
			if err != nil {
				return err
			}

			if reflect.DeepEqual(rule, info) {
				return nil
			}
		}
	}
}

// removeVMAntiAffinityRule removes an anti affinity rule with the name in the given cluster.
func removeVMAntiAffinityRule(ctx context.Context, session *Session, clusterPath string, name string) error {
	cluster, err := session.Finder.ClusterComputeResource(ctx, clusterPath)
	if err != nil {
		return err
	}

	info, err := findClusterAntiAffinityRuleByName(ctx, cluster, name)
	if err != nil {
		return err
	}

	// no rule found
	if info == nil {
		return nil
	}

	spec := &types.ClusterConfigSpecEx{
		RulesSpec: []types.ClusterRuleSpec{
			{
				ArrayUpdateSpec: types.ArrayUpdateSpec{
					Operation: types.ArrayUpdateOperationRemove,
					RemoveKey: info.Key,
				},
			},
		},
	}

	task, err := cluster.Reconfigure(ctx, spec, true)
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

func findClusterAntiAffinityRuleByName(ctx context.Context, cluster *object.ClusterComputeResource, name string) (*types.ClusterAntiAffinityRuleSpec, error) {
	var props mo.ClusterComputeResource
	if err := cluster.Properties(ctx, cluster.Reference(), nil, &props); err != nil {
		return nil, err
	}

	var info *types.ClusterAntiAffinityRuleSpec
	for _, clusterRuleInfo := range props.ConfigurationEx.(*types.ClusterConfigInfoEx).Rule {
		if clusterRuleInfo.GetClusterRuleInfo().Name == name {
			if vmAffinityRuleInfo, ok := clusterRuleInfo.(*types.ClusterAntiAffinityRuleSpec); ok {
				info = vmAffinityRuleInfo
				break
			}
			return nil, fmt.Errorf("rule name %s in cluster %q is not a VM anti-affinity rule", name, cluster.Name())
		}
	}

	return info, nil
}
