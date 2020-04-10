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
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/vmware/govmomi/simulator"
	"k8s.io/apimachinery/pkg/runtime"

	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type machineSpecArgs struct {
	datastore        *string
	datastoreCluster *string
}

func (m machineSpecArgs) generateMachineSpec(user, password, url string) v1alpha1.MachineSpec {
	dsSpec := ""
	if m.datastore != nil {
		dsSpec = fmt.Sprintf(`"datastore": "%s",`, *m.datastore)
	}
	if m.datastoreCluster != nil {
		dsSpec = dsSpec + fmt.Sprintf(`"datastoreCluster": "%s",`, *m.datastoreCluster)
	}
	return v1alpha1.MachineSpec{
		ProviderSpec: v1alpha1.ProviderSpec{
			Value: &runtime.RawExtension{
				Raw: []byte(fmt.Sprintf(`{
					"cloudProvider": "vsphere",
					"cloudProviderSpec": {
						"allowInsecure": false,
						"cluster": "DC0_C0",
						"cpus": 1,
						"datacenter": "DC0",
						%s
						"folder": "/",
						"resourcePool": "/DC0/host/DC0_C0/Resources",
						"memoryMB": 2000,
						"password": "%s",
						"templateVMName": "DC0_H0_VM0",
						"username": "%s",
						"vmNetName": "",
						"vsphereURL": "%s"
					},
					"operatingSystem": "coreos",
					"operatingSystemSpec": {
						"disableAutoUpdate": false,
						"disableLocksmithD": true,
						"disableUpdateEngine": false
					}
				}`, dsSpec, password, user, url)),
			},
		},
	}
}
func Test_provider_Validate(t *testing.T) {
	toPointer := func(s string) *string {
		return &s
	}
	tests := []struct {
		name         string
		args         machineSpecArgs
		getConfigErr error
		wantErr      bool
	}{
		{
			name: "Valid Datastore",
			args: machineSpecArgs{
				datastore: toPointer("LocalDS_0"),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Valid Datastore end empty DatastoreCluster",
			args: machineSpecArgs{
				datastore:        toPointer("LocalDS_0"),
				datastoreCluster: toPointer(""),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Valid DatastoreCluster",
			args: machineSpecArgs{
				datastoreCluster: toPointer("DC0_POD0"),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Invalid Datastore",
			args: machineSpecArgs{
				datastore: toPointer("LocalDS_10"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Invalid DatastoreCluster",
			args: machineSpecArgs{
				datastore: toPointer("DC0_POD10"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Both Datastore and DatastoreCluster specified",
			args: machineSpecArgs{
				datastore:        toPointer("DC0_POD10"),
				datastoreCluster: toPointer("DC0_POD0"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name:         "Neither Datastore nor DatastoreCluster specified",
			args:         machineSpecArgs{},
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
			vSphereURL := strings.TrimSuffix(s.URL.String(), "/sdk")
			username := simulator.DefaultLogin.Username()
			password, _ := simulator.DefaultLogin.Password()
			p := &provider{
				// Note that configVarResolver is not used in this test as the getConfigFunc is mocked.
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), fakeclient.NewFakeClient()),
			}
			if err := p.Validate(tt.args.generateMachineSpec(username, password, vSphereURL)); (err != nil) != tt.wantErr {
				t.Errorf("provider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
