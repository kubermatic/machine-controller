package centos

import (
	"flag"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"github.com/pmezard/go-difflib/difflib"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var pemCertificate = `-----BEGIN CERTIFICATE-----
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

type fakeCloudConfigProvider struct {
	config string
	name   string
	err    error
}

func (p *fakeCloudConfigProvider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return p.config, p.name, p.err
}

var update = flag.Bool("update", false, "update .golden files")

func TestUserDataGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		spec          clusterv1alpha1.MachineSpec
		clusterDNSIPs []net.IP
		dockerVersion string
	}{
		{
			name: "docker-1.13-kubelet-v1.9-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.9.6",
				},
			},
			dockerVersion: "1.13",
		},
		{
			name: "docker-1.13-kubelet-v1.11-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.11.0",
				},
			},
			dockerVersion: "1.13",
		},
		{
			name: "docker-1.13-kubelet-v1.8-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.8.5",
				},
			},
			dockerVersion: "1.13",
		},
	}

	cloudProvider := &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil}
	kubeconfig := &clientcmdapi.Config{Clusters: map[string]*clientcmdapi.Cluster{
		"": &clientcmdapi.Cluster{Server: "https://server:443", CertificateAuthorityData: []byte(pemCertificate)}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"": &clientcmdapi.AuthInfo{Token: "my-token"}}}
	provider := Provider{}

	for _, test := range tests {
		containerRuntimeInfo := machinesv1alpha1.ContainerRuntimeInfo{Name: "docker", Version: test.dockerVersion}
		emtpyProviderConfig := clusterv1alpha1.ProviderConfig{
			Value: &runtime.RawExtension{}}
		providerConfig, err := providerconfig.AddContainerRuntimeInfoToProviderconfig(emtpyProviderConfig,
			containerRuntimeInfo)
		if err != nil {
			t.Fatalf("Failed to add containerRuntimeInfo to providerconfig: %v", err)
		}
		test.spec.ProviderConfig = *providerConfig
		userdata, err := provider.UserData(test.spec, kubeconfig, cloudProvider, test.clusterDNSIPs)
		if err != nil {
			t.Errorf("error getting userdata: '%v'", err)
		}
		golden := filepath.Join("testdata", test.name+".golden")
		if *update {
			ioutil.WriteFile(golden, []byte(userdata), 0644)
		}
		expected, err := ioutil.ReadFile(golden)
		if err != nil {
			t.Errorf("failed to read .golden file: %v", err)
		}

		if string(expected) != userdata {
			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(expected)),
				B:        difflib.SplitLines(userdata),
				FromFile: "Fixture",
				ToFile:   "Current",
				Context:  3,
			}
			diffStr, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Fatal(err)
			}
			t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
		}
	}
}
