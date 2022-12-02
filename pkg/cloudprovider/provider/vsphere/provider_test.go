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
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"text/template"

	"github.com/vmware/govmomi/simulator"

	cloudprovidertesting "github.com/kubermatic/machine-controller/pkg/cloudprovider/testing"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/utils/pointer"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type vsphereProviderSpecConf struct {
	Datastore        *string
	DatastoreCluster *string
	User             string
	Password         string
	URL              string
}

func (v vsphereProviderSpecConf) rawProviderSpec(t *testing.T) []byte {
	var out bytes.Buffer
	tmpl, err := template.New("test").Parse(`{
	"cloudProvider": "vsphere",
	"cloudProviderSpec": {
		"allowInsecure": false,
		"cpus": 1,
		"datacenter": "DC0",
		{{- if .Datastore }}
		"datastore": "{{ .Datastore }}",
		{{- end }}
		{{- if .DatastoreCluster }}
		"datastoreCluster": "{{ .DatastoreCluster }}",
		{{- end }}
		"folder": "/",
		"resourcePool": "/DC0/host/DC0_C0/Resources",
		"memoryMB": 2000,
		"password": "{{ .Password }}",
		"templateVMName": "DC0_H0_VM0",
		"username": "{{ .User }}",
		"vmNetName": "",
		"vsphereURL": "{{ .URL }}"
	},
	"operatingSystem": "flatcar",
	"operatingSystemSpec": {
		"disableAutoUpdate": false,
		"disableLocksmithD": true,
		"disableUpdateEngine": false
	}
}`)
	if err != nil {
		t.Fatalf("Error occurred while parsing openstack provider spec template: %v", err)
	}
	err = tmpl.Execute(&out, v)
	if err != nil {
		t.Fatalf("Error occurred while executing openstack provider spec template: %v", err)
	}
	t.Logf("Generated providerSpec: %s", out.String())
	return out.Bytes()
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		args         vsphereProviderSpecConf
		getConfigErr error
		wantErr      bool
	}{
		{
			name: "Valid Datastore",
			args: vsphereProviderSpecConf{
				Datastore: pointer.String("LocalDS_0"),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Valid Datastore end empty DatastoreCluster",
			args: vsphereProviderSpecConf{
				Datastore:        pointer.String("LocalDS_0"),
				DatastoreCluster: pointer.String(""),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Valid DatastoreCluster",
			args: vsphereProviderSpecConf{
				DatastoreCluster: pointer.String("DC0_POD0"),
			},
			getConfigErr: nil,
			wantErr:      false,
		},
		{
			name: "Invalid Datastore",
			args: vsphereProviderSpecConf{
				Datastore: pointer.String("LocalDS_10"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Invalid DatastoreCluster",
			args: vsphereProviderSpecConf{
				Datastore: pointer.String("DC0_POD10"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name: "Both Datastore and DatastoreCluster specified",
			args: vsphereProviderSpecConf{
				Datastore:        pointer.String("DC0_POD10"),
				DatastoreCluster: pointer.String("DC0_POD0"),
			},
			getConfigErr: nil,
			wantErr:      true,
		},
		{
			name:         "Neither Datastore nor DatastoreCluster specified",
			args:         vsphereProviderSpecConf{},
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
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), fakectrlruntimeclient.
					NewClientBuilder().
					Build()),
			}
			tt.args.User = username
			tt.args.Password = password
			tt.args.URL = vSphereURL
			m := cloudprovidertesting.Creator{Name: "test", Namespace: "vsphere", ProviderSpecGetter: tt.args.rawProviderSpec}.
				CreateMachine(t)
			if err := p.Validate(context.Background(), m.Spec); (err != nil) != tt.wantErr {
				t.Errorf("provider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
