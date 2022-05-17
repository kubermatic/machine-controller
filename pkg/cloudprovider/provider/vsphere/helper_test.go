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

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

func TestResolveDatastoreRef(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Only Datastore defined",
			config: &Config{
				Datastore: "LocalDS_0",
			},
			wantErr: false,
		},
		{
			name: "Only DatastoreCluster defined",
			config: &Config{
				DatastoreCluster: "DC0_POD0",
			},
			wantErr: false,
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
			csrm := &CustomStorageResourceManager{obj, ds}
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
			defer session.Logout(ctx)
			if err != nil {
				t.Fatalf("error creating session: %v", err)
			}
			dc, err := session.Datacenter.Folders(ctx)
			if err != nil {
				t.Fatalf("error getting datacenter folders: %v", err)
			}
			vmFolder := dc.VmFolder
			vms, err := session.Finder.VirtualMachineList(ctx, "*")
			if err != nil {
				t.Fatalf("error getting virtual machines: %v", err)
			}

			got, err := resolveDatastoreRef(ctx, tt.config, session, vms[2], vmFolder, &types.VirtualMachineCloneSpec{})
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveDatastoreRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got == nil {
				t.Errorf("resolveDatastoreRef() should be not empty")
			}
		})
	}
}

type CustomStorageResourceManager struct {
	*simulator.StorageResourceManager
	ds *simulator.Datastore
}

// RecommendDatastores always return a recommendation for the purposes of the test.
func (c *CustomStorageResourceManager) RecommendDatastores(req *types.RecommendDatastores) soap.HasFault {
	body := &methods.RecommendDatastoresBody{}
	res := &types.RecommendDatastoresResponse{}
	ds := c.ds.Reference()
	res.Returnval.Recommendations = append(res.Returnval.Recommendations, types.ClusterRecommendation{
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
			defer session.Logout(ctx)
			if err != nil {
				t.Fatalf("error creating session: %v", err)
			}

			// Obtain a VM from the simulator
			obj := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
			vm := object.NewVirtualMachine(session.Client.Client, obj.Reference())

			got, err := resolveResourcePoolRef(ctx, tt.config, session, vm)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantResourcePool != (got != nil) {
				t.Errorf("resourcePool = %v, wantResourcePool %v", got, tt.wantResourcePool)
			}
			if tt.wantResourcePool {
				rp := object.NewResourcePool(session.Client.Client, got.Reference())
				n, _ := rp.ObjectName(ctx)
				if e, a := tt.expectedResourcePool, n; e != a {
					t.Errorf("expected resource pool %v but got %+v", e, a)
				}
			}
		})
	}
}
