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

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	vspheretypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/vmware/govmomi/simulator"
)

func Test_provider_Validate(t *testing.T) {
	tests := []struct {
		name         string
		config       *Config
		getConfigErr error
		wantErr      bool
	}{
		{
			name: "Valid Datastore",
			config: &Config{
				Datacenter:     "DC0",
				Cluster:        "DC0_C0",
				Folder:         "/",
				Datastore:      "LocalDS_0",
				TemplateVMName: "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Valid DatastoreCluster",
			config: &Config{
				Datacenter:       "DC0",
				Cluster:          "DC0_C0",
				Folder:           "/",
				DatastoreCluster: "DC0_POD0",
				TemplateVMName:   "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Invalid Datastore",
			config: &Config{
				Datacenter:     "DC0",
				Cluster:        "DC0_C0",
				Folder:         "/",
				Datastore:      "LocalDS_10",
				TemplateVMName: "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Invalid DatastoreCluster",
			config: &Config{
				Datacenter:       "DC0",
				Cluster:          "DC0_C0",
				Folder:           "/",
				DatastoreCluster: "DC0_POD10",
				TemplateVMName:   "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Both Datastore and DatastoreCluster specified",
			config: &Config{
				Datacenter:       "DC0",
				Cluster:          "DC0_C0",
				Folder:           "/",
				Datastore:        "LocalDS_0",
				DatastoreCluster: "DC0_POD0",
				TemplateVMName:   "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Neither Datastore nor DatastoreCluster specified",
			config: &Config{
				Datacenter:     "DC0",
				Cluster:        "DC0_C0",
				Folder:         "/",
				TemplateVMName: "DC0_H0_VM0",
			},
			getConfigErr: nil,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := simulator.VPX()
			// Pod == StoragePod == StorageCluster
			model.Pod++
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
			p := &provider{
				// Note that configVarResolver is not used in this test as the getConfigFunc is mocked.
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), nil),
				getConfigFunc: func(v1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *vspheretypes.RawConfig, error) {
					return tt.config, &providerconfigtypes.Config{OperatingSystem: providerconfigtypes.OperatingSystemCoreos}, nil, tt.getConfigErr
				},
			}
			if err := p.Validate(v1alpha1.MachineSpec{}); (err != nil) != tt.wantErr {
				t.Errorf("provider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
