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

package nodesettings

import (
	"net"
	"testing"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
)

func TestSetNodeSettings(t *testing.T) {
	testCases := []struct {
		name             string
		nodeSettings     NodeSettings
		wantNodeSettings string
	}{
		{
			name: "Empty node settings",
			nodeSettings: NodeSettings{
				ClusterDNSIPs:         []net.IP{},
				HTTPProxy:             "",
				NoProxy:               "",
				InsecureRegistries:    []string{},
				RegistryMirrors:       []string{},
				PauseImage:            "",
				HyperkubeImage:        "",
				KubeletRepository:     "",
				KubeletFeatureGates:   map[string]bool{},
				ContainerRuntime:      containerruntime.Config{},
				ExternalCloudProvider: false,
			},
			wantNodeSettings: `{"containerRuntime":{},"externalCloudProvider":false}`,
		},
		{
			name: "Cluster DNS IPs",
			nodeSettings: NodeSettings{
				ClusterDNSIPs:         []net.IP{net.ParseIP("192.168.10.1"), net.ParseIP("192.168.20.1")},
				HTTPProxy:             "",
				NoProxy:               "",
				InsecureRegistries:    []string{},
				RegistryMirrors:       []string{},
				PauseImage:            "",
				HyperkubeImage:        "",
				KubeletRepository:     "",
				KubeletFeatureGates:   map[string]bool{},
				ContainerRuntime:      containerruntime.Config{},
				ExternalCloudProvider: false,
			},
			wantNodeSettings: `{"clusterDNSIPs":["192.168.10.1","192.168.20.1"],"containerRuntime":{},"externalCloudProvider":false}`,
		},
		{
			name: "Feature gates",
			nodeSettings: NodeSettings{
				ClusterDNSIPs:         []net.IP{},
				HTTPProxy:             "",
				NoProxy:               "",
				InsecureRegistries:    []string{},
				RegistryMirrors:       []string{},
				PauseImage:            "",
				HyperkubeImage:        "",
				KubeletRepository:     "",
				KubeletFeatureGates:   map[string]bool{"CSIMigration": true},
				ContainerRuntime:      containerruntime.Config{},
				ExternalCloudProvider: false,
			},
			wantNodeSettings: `{"kubeletFeatureGates":{"CSIMigration":true},"containerRuntime":{},"externalCloudProvider":false}`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := clusterv1alpha1.Machine{}
			tc.nodeSettings.Set(&m)
			val, ok := m.Annotations[AnnotationNodeSettings]
			if !ok {
				t.Fatalf("node settings annotation not found")
			}
			if val != tc.wantNodeSettings {
				t.Errorf("expected %s but got %s", tc.wantNodeSettings, val)
			}
		})
	}
}
