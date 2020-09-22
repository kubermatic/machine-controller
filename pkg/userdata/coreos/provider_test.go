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

//
// UserData plugin for CentOS.
//

package coreos

import (
	"encoding/json"
	"flag"
	"net"
	"testing"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	testhelper "github.com/kubermatic/machine-controller/pkg/test"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"
)

var (
	update = flag.Bool("update", false, "update testdata files")

	pemCertificate = `-----BEGIN CERTIFICATE-----
MIIEWjCCA0KgAwIBAgIJALfRlWsI8YQHMA0GCSqGSIb3DQEBBQUAMHsxCzAJBgNV
BAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEUMBIG
A1UEChMLQnJhZGZpdHppbmMxEjAQBgNVBAMTCWxvY2FsaG9zdDEdMBsGCSqGSIb3
DQEJARYOYnJhZEBkYW5nYS5jb20wHhcNMTQwNzE1MjA0NjA1WhcNMTcwNTA0MjA0
NjA1WjB7MQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAUBgNVBAcTDVNhbiBG
cmFuY2lzY28xFDASBgNVBAoTC0JyYWRmaXR6aW5jMRIwEAYDVQQDEwlsb2NhbGhv
c3QxHTAbBgkqhkiG9w0BCQEWDmJyYWRAZGFuZ2EuY29tMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEAt5fAjp4fTcekWUTfzsp0kyih1OYbsGL0KX1eRbSS
R8Od0+9Q62Hyny+GFwMTb4A/KU8mssoHvcceSAAbwfbxFK/+s51TobqUnORZrOoT
ZjkUygbyXDSK99YBbcR1Pip8vwMTm4XKuLtCigeBBdjjAQdgUO28LENGlsMnmeYk
JfODVGnVmr5Ltb9ANA8IKyTfsnHJ4iOCS/PlPbUj2q7YnoVLposUBMlgUb/CykX3
mOoLb4yJJQyA/iST6ZxiIEj36D4yWZ5lg7YJl+UiiBQHGCnPdGyipqV06ex0heYW
caiW8LWZSUQ93jQ+WVCH8hT7DQO1dmsvUmXlq/JeAlwQ/QIDAQABo4HgMIHdMB0G
A1UdDgQWBBRcAROthS4P4U7vTfjByC569R7E6DCBrQYDVR0jBIGlMIGigBRcAROt
hS4P4U7vTfjByC569R7E6KF/pH0wezELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNB
MRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMRQwEgYDVQQKEwtCcmFkZml0emluYzES
MBAGA1UEAxMJbG9jYWxob3N0MR0wGwYJKoZIhvcNAQkBFg5icmFkQGRhbmdhLmNv
bYIJALfRlWsI8YQHMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAG6h
U9f9sNH0/6oBbGGy2EVU0UgITUQIrFWo9rFkrW5k/XkDjQm+3lzjT0iGR4IxE/Ao
eU6sQhua7wrWeFEn47GL98lnCsJdD7oZNhFmQ95Tb/LnDUjs5Yj9brP0NWzXfYU4
UK2ZnINJRcJpB8iRCaCxE8DdcUF0XqIEq6pA272snoLmiXLMvNl3kYEdm+je6voD
58SNVEUsztzQyXmJEhCpwVI0A6QCjzXj+qvpmw3ZZHi8JwXei8ZZBLTSFBki8Z7n
sH9BBH38/SzUmAN4QHSPy1gjqm00OAE8NaYDkh/bzE4d7mLGGMWp/WE3KPSu82HF
kPe6XoSbiLm/kxk32T0=
-----END CERTIFICATE-----`

	kubeconfig = &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"": {
				Server:                   "https://server:443",
				CertificateAuthorityData: []byte(pemCertificate),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"": {
				Token: "my-token",
			},
		},
	}
)

// fakeCloudConfigProvider simulates cloud config provider for test.
type fakeCloudConfigProvider struct {
	config string
	name   string
	err    error
}

func (p *fakeCloudConfigProvider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return p.config, p.name, p.err
}

// userDataTestCase contains the data for a table-driven test.
type userDataTestCase struct {
	name                  string
	spec                  clusterv1alpha1.MachineSpec
	ccProvider            cloud.ConfigProvider
	osConfig              *Config
	providerSpec          *providerconfigtypes.Config
	DNSIPs                []net.IP
	externalCloudProvider bool
	httpProxy             string
	noProxy               string
	insecureRegistries    []string
	registryMirrors       []string
	pauseImage            string
	hyperkubeImage        string
}

// TestUserDataGeneration runs the data generation for different
// environments.
func TestUserDataGeneration(t *testing.T) {
	t.Parallel()

	tests := []userDataTestCase{
		{
			name: "v1.17.2-disable-auto-update-aws",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "aws",
				config: "{aws-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
		},
		{
			name: "v1.17.2-disable-locksmith-aws",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "aws",
				config: "{aws-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableLocksmithD: true,
			},
		},
		{
			name: "v1.17.2-disable-update-engine-aws",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "aws",
				config: "{aws-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableUpdateEngine: true,
			},
		},
		{
			name: "v1.17.2-disable-auto-update-aws-external",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "aws",
				config: "{aws-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
			externalCloudProvider: true,
		},
		{
			name: "v1.17.3-auto-update-openstack-multiple-dns",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "openstack",
				config: "{openstack-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10"), net.ParseIP("10.10.10.11"), net.ParseIP("10.10.10.12")},
			osConfig: &Config{
				DisableAutoUpdate: false,
			},
		},
		{
			name: "auto-update-openstack-kubelet-v-version-prefix",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.17.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "openstack",
				config: "{openstack-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: false,
			},
		},
		{
			name: "v1.16.2-vsphere-static-ipconfig",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "vsphere",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfigtypes.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfigtypes.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.16.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
		},
		{
			name: "v1.17.0-vsphere-overwrite-cloudconfig",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider:        "vsphere",
				OverwriteCloudConfig: stringPtr("my\ncustom\ncloud-config"),
				SSHPublicKeys:        []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfigtypes.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfigtypes.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.17.0",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
		},
		{
			name: "v1.15.0-vsphere",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "vsphere",
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.15.0-beta.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
		},
		{
			name: "v1.17.0-vsphere-proxy",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "vsphere",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfigtypes.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfigtypes.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.17.0",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
			httpProxy:          "http://192.168.100.100:3128",
			noProxy:            "192.168.1.0",
			insecureRegistries: []string{"192.168.100.100:5000", "10.0.0.1:5000"},
			pauseImage:         "192.168.100.100:5000/kubernetes/pause:v3.1",
			hyperkubeImage:     "192.168.100.100:5000/kubernetes/hyperkube",
		},
		{
			name: "v1.17.0-vsphere-mirrors",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "vsphere",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfigtypes.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfigtypes.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.17.0",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
			httpProxy:       "http://192.168.100.100:3128",
			noProxy:         "192.168.1.0",
			registryMirrors: []string{"https://registry.docker-cn.com"},
			pauseImage:      "192.168.100.100:5000/kubernetes/pause:v3.1",
			hyperkubeImage:  "192.168.100.100:5000/kubernetes/hyperkube",
		},
		{
			name: "v1.17.0",
			providerSpec: &providerconfigtypes.Config{
				CloudProvider: "vsphere",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfigtypes.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfigtypes.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.17.0",
				},
			},
			ccProvider: &fakeCloudConfigProvider{
				name:   "vsphere",
				config: "{vsphere-config:true}",
				err:    nil,
			},
			DNSIPs: []net.IP{net.ParseIP("10.10.10.10")},
			osConfig: &Config{
				DisableAutoUpdate: true,
			},
		},
	}

	kubeletFeatureGates := map[string]bool{
		"RotateKubeletServerCertificate": true,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rProviderSpec := test.providerSpec
			osConfigByte, err := json.Marshal(test.osConfig)
			if err != nil {
				t.Fatal(err)
			}
			rProviderSpec.OperatingSystemSpec = runtime.RawExtension{
				Raw: osConfigByte,
			}

			providerSpecRaw, err := json.Marshal(rProviderSpec)
			if err != nil {
				t.Fatal(err)
			}
			test.spec.ProviderSpec = clusterv1alpha1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: providerSpecRaw,
				},
			}
			provider := Provider{}

			cloudConfig, cloudProviderName, err := test.ccProvider.GetCloudConfig(test.spec)
			if err != nil {
				t.Fatalf("failed to get cloud config: %v", err)
			}

			req := plugin.UserDataRequest{
				MachineSpec:           test.spec,
				Kubeconfig:            kubeconfig,
				CloudConfig:           cloudConfig,
				CloudProviderName:     cloudProviderName,
				DNSIPs:                test.DNSIPs,
				ExternalCloudProvider: test.externalCloudProvider,
				HTTPProxy:             test.httpProxy,
				NoProxy:               test.noProxy,
				InsecureRegistries:    test.insecureRegistries,
				RegistryMirrors:       test.registryMirrors,
				PauseImage:            test.pauseImage,
				HyperkubeImage:        test.hyperkubeImage,
				KubeletFeatureGates:   kubeletFeatureGates,
			}

			s, err := provider.UserData(req)
			if err != nil {
				t.Fatal(err)
			}

			// Check if we can convert it to ignition.
			if _, err := convert.ToIgnition(s); err != nil {
				t.Fatal(err)
			}
			goldenName := test.name + ".yaml"
			testhelper.CompareOutput(t, goldenName, s, *update)
		})
	}
}

// stringPtr returns pointer to given string.
func stringPtr(str string) *string {
	return &str
}
