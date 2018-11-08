package coreos

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/userdata/convert"

	"github.com/pmezard/go-difflib/difflib"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var (
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
			"": &clientcmdapi.Cluster{
				Server:                   "https://server:443",
				CertificateAuthorityData: []byte(pemCertificate),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"": &clientcmdapi.AuthInfo{
				Token: "my-token",
			},
		},
	}
)

type fakeCloudConfigProvider struct {
	config string
	name   string
	err    error
}

func (p *fakeCloudConfigProvider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return p.config, p.name, p.err
}

var update = flag.Bool("update", false, "update .golden files")

func TestProvider_UserData(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		spec             clusterv1alpha1.MachineSpec
		ccProvider       cloud.ConfigProvider
		osConfig         *Config
		providerConfig   *providerconfig.Config
		DNSIPs           []net.IP
		kubernetesCACert string
	}{
		{
			name: "v1.9.2-disable-auto-update-aws",
			providerConfig: &providerconfig.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			osConfig:         &Config{DisableAutoUpdate: true},
		},
		{
			name: "v1.10.3-auto-update-openstack-multiple-dns",
			providerConfig: &providerconfig.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.10.3",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10"), net.ParseIP("10.10.10.11"), net.ParseIP("10.10.10.12")},
			kubernetesCACert: "CACert",
			osConfig:         &Config{DisableAutoUpdate: false},
		},
		{
			name: "auto-update-openstack-kubelet-v-version-prefix",
			providerConfig: &providerconfig.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			osConfig:         &Config{DisableAutoUpdate: false},
		},
		{
			name: "v1.11.2-vsphere-static-ipconfig",
			providerConfig: &providerconfig.Config{
				CloudProvider: "vsphere",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfig.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfig.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.11.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "vsphere", config: "{vsphere-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			osConfig:         &Config{DisableAutoUpdate: true},
		},
		{
			name: "v1.12.0-vsphere-overwrite-cloudconfig",
			providerConfig: &providerconfig.Config{
				CloudProvider:        "vsphere",
				OverwriteCloudConfig: stringPtr("my\ncustom\ncloud-config"),
				SSHPublicKeys:        []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
				Network: &providerconfig.NetworkConfig{
					CIDR:    "192.168.81.4/24",
					Gateway: "192.168.81.1",
					DNS: providerconfig.DNSConfig{
						Servers: []string{"8.8.8.8"},
					},
				},
			},
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "v1.12.0",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "vsphere", config: "{vsphere-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			osConfig:         &Config{DisableAutoUpdate: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := test.spec
			rProviderConfig := test.providerConfig
			osConfigByte, err := json.Marshal(test.osConfig)
			if err != nil {
				t.Fatal(err)
			}
			rProviderConfig.OperatingSystemSpec = runtime.RawExtension{Raw: osConfigByte}

			providerConfigRaw, err := json.Marshal(rProviderConfig)
			if err != nil {
				t.Fatal(err)
			}
			spec.ProviderConfig = clusterv1alpha1.ProviderConfig{Value: &runtime.RawExtension{Raw: providerConfigRaw}}
			p := Provider{}

			s, err := p.UserData(spec, kubeconfig, test.ccProvider, test.DNSIPs)
			if err != nil {
				t.Fatal(err)
			}

			// Check if we can convert it to ignition
			if _, err := convert.ToIgnition(s); err != nil {
				t.Fatal(err)
			}

			golden := filepath.Join("testdata", test.name+".golden")
			if *update {
				ioutil.WriteFile(golden, []byte(s), 0644)
			}
			expected, err := ioutil.ReadFile(golden)
			if err != nil {
				t.Errorf("failed to read .golden file: %v", err)
			}

			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(expected)),
				B:        difflib.SplitLines(s),
				FromFile: "Fixture",
				ToFile:   "Current",
				Context:  3,
			}
			diffStr, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Fatal(err)
			}

			if diffStr != "" {
				t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
			}
		})
	}
}

func stringPtr(str string) *string {
	return &str
}
