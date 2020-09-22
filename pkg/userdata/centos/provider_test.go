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

package centos

import (
	"flag"
	"net"
	"testing"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	testhelper "github.com/kubermatic/machine-controller/pkg/test"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
	clusterDNSIPs         []net.IP
	cloudProviderName     *string
	externalCloudProvider bool
	httpProxy             string
	noProxy               string
	insecureRegistries    []string
	registryMirrors       []string
	pauseImage            string
}

// TestUserDataGeneration runs the data generation for different
// environments.
func TestUserDataGeneration(t *testing.T) {
	t.Parallel()

	tests := []userDataTestCase{
		{
			name: "kubelet-v1.15-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.15.10",
				},
			},
		},
		{
			name: "kubelet-v1.16-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.16.6",
				},
			},
		},
		{
			name: "kubelet-v1.17-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
		},
		{
			name: "kubelet-v1.17-aws-external",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
			externalCloudProvider: true,
		},
		{
			name: "kubelet-v1.17-vsphere",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
			cloudProviderName: stringPtr("vsphere"),
		},
		{
			name: "kubelet-v1.17-vsphere-proxy",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
			cloudProviderName:  stringPtr("vsphere"),
			httpProxy:          "http://192.168.100.100:3128",
			noProxy:            "192.168.1.0",
			insecureRegistries: []string{"192.168.100.100:5000", "10.0.0.1:5000"},
			pauseImage:         "192.168.100.100:5000/kubernetes/pause:v3.1",
		},
		{
			name: "kubelet-v1.17-vsphere-mirrors",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.17.3",
				},
			},
			cloudProviderName: stringPtr("vsphere"),
			httpProxy:         "http://192.168.100.100:3128",
			noProxy:           "192.168.1.0",
			registryMirrors:   []string{"https://registry.docker-cn.com"},
			pauseImage:        "192.168.100.100:5000/kubernetes/pause:v3.1",
		},
	}

	defaultCloudProvider := &fakeCloudConfigProvider{
		name:   "aws",
		config: "{aws-config:true}",
		err:    nil,
	}
	kubeconfig := &clientcmdapi.Config{
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
	provider := Provider{}

	kubeletFeatureGates := map[string]bool{
		"RotateKubeletServerCertificate": true,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			emtpyProviderSpec := clusterv1alpha1.ProviderSpec{
				Value: &runtime.RawExtension{},
			}
			test.spec.ProviderSpec = emtpyProviderSpec
			var cloudProvider *fakeCloudConfigProvider
			if test.cloudProviderName != nil {
				cloudProvider = &fakeCloudConfigProvider{
					name:   *test.cloudProviderName,
					config: "{config:true}",
					err:    nil,
				}
			} else {
				cloudProvider = defaultCloudProvider
			}
			cloudConfig, cloudProviderName, err := cloudProvider.GetCloudConfig(test.spec)
			if err != nil {
				t.Fatalf("failed to get cloud config: %v", err)
			}

			req := plugin.UserDataRequest{
				MachineSpec:           test.spec,
				Kubeconfig:            kubeconfig,
				CloudConfig:           cloudConfig,
				CloudProviderName:     cloudProviderName,
				DNSIPs:                test.clusterDNSIPs,
				ExternalCloudProvider: test.externalCloudProvider,
				HTTPProxy:             test.httpProxy,
				NoProxy:               test.noProxy,
				InsecureRegistries:    test.insecureRegistries,
				RegistryMirrors:       test.registryMirrors,
				PauseImage:            test.pauseImage,
				KubeletFeatureGates:   kubeletFeatureGates,
			}
			s, err := provider.UserData(req)
			if err != nil {
				t.Errorf("error getting userdata: '%v'", err)
			}

			// Check if we can gzip it.
			if _, err := convert.GzipString(s); err != nil {
				t.Fatal(err)
			}
			goldenName := test.name + ".yaml"
			testhelper.CompareOutput(t, goldenName, s, *update)
		})
	}
}

// stringPtr returns pointer to given string.
func stringPtr(a string) *string {
	return &a
}
