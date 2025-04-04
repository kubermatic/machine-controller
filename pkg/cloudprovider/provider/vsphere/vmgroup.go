/*
Copyright 2024 The Machine Controller Authors.

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
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"

	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
)

func (p *provider) addToVMGroup(ctx context.Context, log *zap.SugaredLogger, session *Session, machine *clusterv1alpha1.Machine, config *Config) error {
	lock.Lock()
	defer lock.Unlock()

	// Check if the VM group exists
	vmGroup, err := findVMGroup(ctx, session, config.Cluster, config.VMGroup)
	if err != nil {
		return err
	}

	// We have to find all VMs in the folder and add them to the VM group. VMGroup only contains VM reference ID which is not enough to
	// identify the VM by name.
	machineSetName := machine.Name[:strings.LastIndex(machine.Name, "-")]
	vmsInFolder, err := session.Finder.VirtualMachineList(ctx, strings.Join([]string{config.Folder, "*"}, "/"))
	if err != nil {
		return fmt.Errorf("failed to find VMs in folder: %w", err)
	}

	var vmRefs []types.ManagedObjectReference
	for _, vm := range vmsInFolder {
		// Only add VMs with the same machineSetName to the rule and exclude the machine itself if it is being deleted
		if strings.HasPrefix(vm.Name(), machineSetName) && (vm.Name() != machine.Name || machine.DeletionTimestamp == nil) {
			vmRefs = append(vmRefs, vm.Reference())
		}
	}

	var vmRefsToAdd []types.ManagedObjectReference
	for _, vm := range vmRefs {
		found := false
		for _, existingVM := range vmGroup.Vm {
			if existingVM.Value == vm.Value {
				log.Debugf("VM %s already in VM group %s", machine.Name, config.VMGroup)
				found = true
				break
			}
		}
		if !found {
			vmRefsToAdd = append(vmRefsToAdd, vm)
		}
	}

	// Add the VM to the VM group
	vmGroup.Vm = append(vmGroup.Vm, vmRefsToAdd...)
	cluster, err := session.Finder.ClusterComputeResource(ctx, config.Cluster)
	if err != nil {
		return err
	}

	spec := &types.ClusterConfigSpecEx{
		GroupSpec: []types.ClusterGroupSpec{
			{
				ArrayUpdateSpec: types.ArrayUpdateSpec{
					Operation: types.ArrayUpdateOperationEdit,
				},
				Info: vmGroup,
			},
		},
	}

	log.Debugf("Adding VM %s in VM group %s", machine.Name, config.VMGroup)
	task, err := cluster.Reconfigure(ctx, spec, true)
	if err != nil {
		return err
	}

	taskResult, err := task.WaitForResultEx(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for cluster %v reconfiguration to complete", cluster.Name())
	}
	if taskResult.State != types.TaskInfoStateSuccess {
		return fmt.Errorf("cluster %v reconfiguration task was not successful", cluster.Name())
	}
	log.Debugf("Successfully added VM %s in VM group %s", machine.Name, config.VMGroup)
	return nil
}

func findVMGroup(ctx context.Context, session *Session, clusterName, vmGroup string) (*types.ClusterVmGroup, error) {
	cluster, err := session.Finder.ClusterComputeResource(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	clusterConfigInfoEx, err := cluster.Configuration(ctx)
	if err != nil {
		return nil, err
	}

	for _, group := range clusterConfigInfoEx.Group {
		if clusterVMGroup, ok := group.(*types.ClusterVmGroup); ok {
			if clusterVMGroup.Name == vmGroup {
				return clusterVMGroup, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot find VM group %s", vmGroup)
}
