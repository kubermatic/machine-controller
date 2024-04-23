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
	"strings"
	"sync"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	"k8s.io/utils/ptr"
)

var lock sync.Mutex

// createOrUpdateVMAntiAffinityRule creates or updates an anti affinity rule with the name in the given cluster.
// VMs are attached to the rule based on their folder path and name prefix in vsphere.
// A minimum of two VMs is required.
func (p *provider) createOrUpdateVMAntiAffinityRule(ctx context.Context, log *zap.SugaredLogger, session *Session, machine *clusterv1alpha1.Machine, config *Config) error {
	lock.Lock()
	defer lock.Unlock()
	cluster, err := session.Finder.ClusterComputeResource(ctx, config.Cluster)
	if err != nil {
		return err
	}

	machineSetName := machine.Name[:strings.LastIndex(machine.Name, "-")]
	vmsInFolder, err := session.Finder.VirtualMachineList(ctx, strings.Join([]string{config.Folder, "*"}, "/"))
	if err != nil {
		if errors.Is(err, &find.NotFoundError{}) {
			return removeVMAntiAffinityRule(ctx, session, config.Cluster, machineSetName)
		}
		return err
	}

	var ruleVMRef []types.ManagedObjectReference
	for _, vm := range vmsInFolder {
		// Only add VMs with the same machineSetName to the rule and exclude the machine itself if it is being deleted
		if strings.HasPrefix(vm.Name(), machineSetName) && !(vm.Name() == machine.Name && machine.DeletionTimestamp != nil) {
			ruleVMRef = append(ruleVMRef, vm.Reference())
		}
	}

	if len(ruleVMRef) == 0 {
		log.Debugf("No VMs in folder %s with name prefix %s found", config.Folder, machineSetName)
		return removeVMAntiAffinityRule(ctx, session, config.Cluster, machineSetName)
	} else if len(ruleVMRef) < 2 {
		// DRS rule must have at least two virtual machine members
		log.Debugf("Not enough VMs in folder %s to create anti-affinity rule", config.Folder)
		return nil
	}

	info, err := findClusterAntiAffinityRuleByName(ctx, cluster, machineSetName)
	if err != nil {
		return err
	}

	log.Debugf("Creating or updating anti-affinity rule for VMs %v in cluster %s", ruleVMRef, config.Cluster)
	operation := types.ArrayUpdateOperationEdit

	//create new rule
	if info == nil {
		info = &types.ClusterAntiAffinityRuleSpec{
			ClusterRuleInfo: types.ClusterRuleInfo{
				Enabled:     ptr.To(true),
				Mandatory:   ptr.To(false),
				Name:        machineSetName,
				UserCreated: ptr.To(true),
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

	log.Debugf("Performing %q for anti-affinity rule for VMs %v in cluster %s", operation, ruleVMRef, config.Cluster)
	task, err := cluster.Reconfigure(ctx, spec, true)
	if err != nil {
		return err
	}

	taskResult, err := task.WaitForResult(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for cluster %v reconfiguration to complete", cluster.Name())
	}
	if taskResult.State != types.TaskInfoStateSuccess {
		return fmt.Errorf("cluster %v reconfiguration task was not successful", cluster.Name())
	}
	log.Debugf("Successfully created/updated anti-affinity rule for machineset %v against machine %v", machineSetName, machine.Name)

	return nil
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

	taskResult, err := task.WaitForResult(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for cluster %v reconfiguration to complete", cluster.Name())
	}
	if taskResult.State != types.TaskInfoStateSuccess {
		return fmt.Errorf("cluster %v reconfiguration task was not successful", cluster.Name())
	}
	return nil
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
