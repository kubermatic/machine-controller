/*
Copyright 2026 The Machine Controller Authors.

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

package hetzner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"

	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	hetznertypes "k8c.io/machine-controller/sdk/cloudprovider/hetzner"
	"k8c.io/machine-controller/sdk/providerconfig"
	"k8c.io/machine-controller/sdk/providerconfig/configvar"

	"k8s.io/apimachinery/pkg/runtime"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDatacenterToLocation(t *testing.T) {
	tests := []struct {
		datacenter   string
		wantLocation string
		wantErr      string
	}{
		{datacenter: "nbg1-dc3", wantLocation: "nbg1"},
		{datacenter: "fsn1-dc14", wantLocation: "fsn1"},
		{datacenter: "ash-dc1", wantLocation: "ash"},
		{datacenter: "hel1-dc2", wantLocation: "hel1"},
		{datacenter: "4", wantErr: "is a numeric ID"},
	}
	for _, tt := range tests {
		t.Run(tt.datacenter, func(t *testing.T) {
			location, err := datacenterToLocation(tt.datacenter)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("datacenterToLocation(%q) error = %v, want error containing %q", tt.datacenter, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("datacenterToLocation(%q) error = %v", tt.datacenter, err)
			}
			if location != tt.wantLocation {
				t.Fatalf("datacenterToLocation(%q) = %q, want %q", tt.datacenter, location, tt.wantLocation)
			}
		})
	}
}

func hetznerProviderSpec(t *testing.T, datacenter, location string) clusterv1alpha1.ProviderSpec {
	t.Helper()

	rawConfig := hetznertypes.RawConfig{
		Token:      providerconfig.ConfigVarString{Value: "fake-token"},
		ServerType: providerconfig.ConfigVarString{Value: "cx22"},
		Datacenter: providerconfig.ConfigVarString{Value: datacenter},
		Location:   providerconfig.ConfigVarString{Value: location},
	}
	rawCloudProviderSpec, err := json.Marshal(rawConfig)
	if err != nil {
		t.Fatalf("failed to marshal hetzner raw config: %v", err)
	}

	pconfig := providerconfig.Config{
		CloudProvider:       providerconfig.CloudProviderHetzner,
		CloudProviderSpec:   runtime.RawExtension{Raw: rawCloudProviderSpec},
		OperatingSystem:     providerconfig.OperatingSystemUbuntu,
		OperatingSystemSpec: runtime.RawExtension{Raw: []byte("{}")},
	}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		t.Fatalf("failed to marshal providerconfig: %v", err)
	}

	return clusterv1alpha1.ProviderSpec{Value: &runtime.RawExtension{Raw: rawPconfig}}
}

func hetznerRawConfig(t *testing.T, spec clusterv1alpha1.MachineSpec) *hetznertypes.RawConfig {
	t.Helper()

	pconfig, err := providerconfig.GetConfig(spec.ProviderSpec)
	if err != nil {
		t.Fatalf("failed to read providerconfig from defaulted spec: %v", err)
	}
	rawConfig, err := hetznertypes.GetConfig(*pconfig)
	if err != nil {
		t.Fatalf("failed to read hetzner raw config from defaulted spec: %v", err)
	}
	return rawConfig
}

func TestAddDefaultsMigratesDatacenterToLocation(t *testing.T) {
	tests := []struct {
		name           string
		datacenter     string
		location       string
		wantDatacenter string
		wantLocation   string
		wantErr        string
	}{
		{
			name:         "datacenter is migrated to location and cleared",
			datacenter:   "nbg1-dc3",
			wantLocation: "nbg1",
		},
		{
			name:       "numeric datacenter ID is rejected",
			datacenter: "4",
			wantErr:    "is a numeric ID",
		},
		{
			name:       "both set is rejected",
			datacenter: "nbg1-dc3",
			location:   "fsn1",
			wantErr:    "location and datacenter must not be set at the same time",
		},
		{
			name: "both empty is a no-op",
		},
		{
			name:         "location only is a no-op",
			location:     "fsn1",
			wantLocation: "fsn1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &provider{
				configVarResolver: configvar.NewResolver(context.Background(), fakectrlruntimeclient.NewClientBuilder().Build()),
			}
			spec := clusterv1alpha1.MachineSpec{
				ProviderSpec: hetznerProviderSpec(t, tt.datacenter, tt.location),
			}

			defaultedSpec, err := p.AddDefaults(zap.NewNop().Sugar(), spec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("AddDefaults() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AddDefaults() error = %v", err)
			}

			rawConfig := hetznerRawConfig(t, defaultedSpec)
			if rawConfig.Datacenter.Value != tt.wantDatacenter {
				t.Errorf("datacenter = %q, want %q", rawConfig.Datacenter.Value, tt.wantDatacenter)
			}
			if rawConfig.Location.Value != tt.wantLocation {
				t.Errorf("location = %q, want %q", rawConfig.Location.Value, tt.wantLocation)
			}
		})
	}
}
