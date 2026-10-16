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

package vsphere

import (
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

func TestResolveDatastoreRef(t *testing.T) {
	const (
		// The template VM used by default lives in cluster DC0_C0.
		defaultTemplateVM = "DC0_C0_RP0_VM0"
		c0RootPool        = "/DC0/host/DC0_C0/Resources"
		c1RootPool        = "/DC0/host/DC0_C1/Resources"
	)

	tests := []struct {
		name   string
		config *Config
		// markAsTemplate marks the source VM as a template, which has no resource pool.
		markAsTemplate bool
		// noActions makes the storage resource manager return a recommendation
		// without any actions.
		noActions bool
		// expectedPool is the inventory path of the resource pool expected on the
		// clone spec, empty if no pool is expected.
		expectedPool string
		wantErr      bool
	}{
		{
			name: "Only Datastore defined",
			config: &Config{
				Datastore: "LocalDS_0",
			},
			expectedPool: "",
			wantErr:      false,
		},
		{
			name: "Datastore with Cluster different from the template's",
			config: &Config{
				Datastore: "LocalDS_0",
				Cluster:   "DC0_C1",
			},
			expectedPool: c1RootPool,
			wantErr:      false,
		},
		{
			name: "Only DatastoreCluster defined",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
			},
			expectedPool: c0RootPool,
			wantErr:      false,
		},
		{
			name: "DatastoreCluster with template as source",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
			},
			markAsTemplate: true,
			expectedPool:   c0RootPool,
			wantErr:        false,
		},
		{
			name: "DatastoreCluster with ResourcePool",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
				ResourcePool:     c1RootPool,
			},
			expectedPool: c1RootPool,
			wantErr:      false,
		},
		{
			name: "DatastoreCluster with Cluster different from the template's",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
				Cluster:          "DC0_C1",
			},
			expectedPool: c1RootPool,
			wantErr:      false,
		},
		{
			name: "DatastoreCluster with Cluster different from the template's and template as source",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
				Cluster:          "DC0_C1",
			},
			markAsTemplate: true,
			expectedPool:   c1RootPool,
			wantErr:        false,
		},
		{
			name: "DatastoreCluster recommendation without actions",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
			},
			noActions: true,
			wantErr:   true,
		},
		{
			name: "Unknown DatastoreCluster",
			config: &Config{
				DatastoreCluster: "DC0_POD1",
			},
			wantErr: true,
		},
		{
			name: "Both Datastore and DatastoreCluster defined",
			config: &Config{
				Datastore:        "LocalDS_0",
				DatastoreCluster: "DC0_POD0",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			model := simulator.VPX()
			// Pod == StoragePod == StorageCluster
			model.Pod++
			model.Cluster++

			defer model.Remove()
			err := model.Create()
			if err != nil {
				log.Fatal(err)
			}

			// Override the default StorageResourceManager for the purpose of the unit test.
			ds := simulator.Map.Any("Datastore").(*simulator.Datastore)
			obj := simulator.Map.Get(model.ServiceContent.StorageResourceManager.Reference()).(*simulator.StorageResourceManager)
			csrm := &CustomStorageResourceManager{StorageResourceManager: obj, ds: ds, noActions: tt.noActions}
			simulator.Map.Put(csrm)

			s := model.Service.NewServer()
			defer s.Close()

			// Setup config to be able to login to the simulator
			// Remove trailing `/sdk` as it is appended by the session constructor
			tt.config.VSphereURL = strings.TrimSuffix(s.URL.String(), "/sdk")
			tt.config.Username = simulator.DefaultLogin.Username()
			tt.config.Password, _ = simulator.DefaultLogin.Password()
			tt.config.Datacenter = "DC0"

			session, err := NewSession(ctx, tt.config)
			if err != nil {
				t.Fatalf("error creating session: %v", err)
			}
			defer session.Logout(ctx)
			dc, err := session.Datacenter.Folders(ctx)
			if err != nil {
				t.Fatalf("error getting datacenter folders: %v", err)
			}
			vmFolder := dc.VmFolder
			vm, err := session.Finder.VirtualMachine(ctx, defaultTemplateVM)
			if err != nil {
				t.Fatalf("error getting virtual machine: %v", err)
			}
			if tt.markAsTemplate {
				task, err := vm.PowerOff(ctx)
				if err != nil {
					t.Fatalf("error powering off vm: %v", err)
				}
				if err := task.WaitEx(ctx); err != nil {
					t.Fatalf("error waiting for vm power off: %v", err)
				}
				if err := vm.MarkAsTemplate(ctx); err != nil {
					t.Fatalf("error marking vm as template: %v", err)
				}
			}

			// Resolve the resource pool before the datastore, like createClonedVM does.
			cloneSpec := &types.VirtualMachineCloneSpec{}
			cloneSpec.Location.Pool, err = resolveResourcePoolRef(ctx, tt.config, session)
			if err != nil {
				t.Fatalf("error resolving resource pool: %v", err)
			}

			got, err := resolveDatastoreRef(ctx, zap.NewNop().Sugar(), tt.config, session, vm, vmFolder, cloneSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveDatastoreRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got == nil {
				t.Errorf("resolveDatastoreRef() should be not empty")
			}

			if tt.expectedPool == "" {
				if cloneSpec.Location.Pool != nil {
					t.Errorf("expected no resource pool, got %v", cloneSpec.Location.Pool)
				}
				return
			}
			expectedPool, err := session.Finder.ResourcePool(ctx, tt.expectedPool)
			if err != nil {
				t.Fatalf("error getting expected resource pool: %v", err)
			}
			if cloneSpec.Location.Pool == nil || *cloneSpec.Location.Pool != expectedPool.Reference() {
				t.Errorf("expected resource pool %v (%s), got %v", expectedPool.Reference(), tt.expectedPool, cloneSpec.Location.Pool)
			}
		})
	}
}

type CustomStorageResourceManager struct {
	*simulator.StorageResourceManager
	ds        *simulator.Datastore
	noActions bool
}

// RecommendDatastores always return a recommendation for the purposes of the test,
// as long as the clone spec contains a resource pool or a host, like vCenter requires.
func (c *CustomStorageResourceManager) RecommendDatastores(req *types.RecommendDatastores) soap.HasFault {
	body := &methods.RecommendDatastoresBody{}

	if spec := req.StorageSpec.CloneSpec; spec == nil || (spec.Location.Pool == nil && spec.Location.Host == nil) {
		body.Fault_ = simulator.Fault("", &types.InvalidArgument{InvalidProperty: "spec.host"})
		return body
	}

	res := &types.RecommendDatastoresResponse{}
	if c.noActions {
		res.Returnval.Recommendations = append(res.Returnval.Recommendations, types.ClusterRecommendation{
			Key:    "0",
			Type:   "V1",
			Time:   time.Now(),
			Reason: "storagePlacement",
		})
		body.Res = res
		return body
	}

	ds := c.ds.Reference()
	res.Returnval.Recommendations = append(
		res.Returnval.Recommendations, types.ClusterRecommendation{
			Key:            "0",
			Type:           "V1",
			Time:           time.Now(),
			Reason:         "storagePlacement",
			ReasonText:     "Satisfy storage initial placement requests",
			WarningDetails: (*types.LocalizableMessage)(nil),
			Prerequisite:   nil,
			Action: []types.BaseClusterAction{
				&types.StoragePlacementAction{
					ClusterAction: types.ClusterAction{
						Type:   "StoragePlacementV1",
						Target: (*types.ManagedObjectReference)(nil),
					},
					Vm:          (*types.ManagedObjectReference)(nil),
					Destination: ds,
				},
			},
		},
	)

	body.Res = res
	return body
}

func TestResolveResourcePoolRef(t *testing.T) {
	tests := []struct {
		name                 string
		config               *Config
		wantErr              bool
		wantResourcePool     bool
		expectedResourcePool string
	}{
		{
			name:             "No Resource Pool specified",
			config:           &Config{},
			wantErr:          false,
			wantResourcePool: false,
		},
		{
			name: "Resource Pool specified",
			config: &Config{
				ResourcePool: "DC0_C0_RP1",
			},
			wantErr:              false,
			wantResourcePool:     true,
			expectedResourcePool: "DC0_C0_RP1",
		},
		{
			name: "Resource Pool specified missing",
			config: &Config{
				ResourcePool: "DC0_C0_RP1_WRONG",
			},
			wantErr:          true,
			wantResourcePool: false,
		},
		{
			name: "Cluster specified without Resource Pool",
			config: &Config{
				Cluster: "DC0_C0",
			},
			wantErr:              false,
			wantResourcePool:     true,
			expectedResourcePool: "/DC0/host/DC0_C0/Resources",
		},
		{
			name: "Unknown Cluster",
			config: &Config{
				Cluster: "DC0_C0_WRONG",
			},
			wantErr:          true,
			wantResourcePool: false,
		},
		{
			name: "Resource Pool takes precedence over Cluster",
			config: &Config{
				Cluster:      "DC0_C0",
				ResourcePool: "DC0_C0_RP1",
			},
			wantErr:              false,
			wantResourcePool:     true,
			expectedResourcePool: "DC0_C0_RP1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			model := simulator.VPX()
			model.Pool++
			model.Cluster++

			defer model.Remove()
			err := model.Create()
			if err != nil {
				log.Fatal(err)
			}

			s := model.Service.NewServer()
			defer s.Close()

			// Setup config to be able to login to the simulator
			// Remove trailing `/sdk` as it is appended by the session constructor
			tt.config.VSphereURL = strings.TrimSuffix(s.URL.String(), "/sdk")
			tt.config.Username = simulator.DefaultLogin.Username()
			tt.config.Password, _ = simulator.DefaultLogin.Password()
			tt.config.Datacenter = "DC0"

			session, err := NewSession(ctx, tt.config)
			if err != nil {
				t.Fatalf("error creating session: %v", err)
			}
			defer session.Logout(ctx)

			got, err := resolveResourcePoolRef(ctx, tt.config, session)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantResourcePool != (got != nil) {
				t.Errorf("resourcePool = %v, wantResourcePool %v", got, tt.wantResourcePool)
			}
			if tt.wantResourcePool && got != nil {
				// Compare references rather than names, as every root resource pool is named "Resources".
				expected, err := session.Finder.ResourcePool(ctx, tt.expectedResourcePool)
				if err != nil {
					t.Fatalf("error getting expected resource pool: %v", err)
				}
				if *got != expected.Reference() {
					t.Errorf("expected resource pool %v (%s) but got %v", expected.Reference(), tt.expectedResourcePool, got)
				}
			}
		})
	}
}
