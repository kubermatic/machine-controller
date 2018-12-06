package centos

import (
	"flag"
	"net"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/userdata/convert"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		name              string
		spec              clusterv1alpha1.MachineSpec
		clusterDNSIPs     []net.IP
		cloudProviderName *string
	}{
		{
			name: "kubelet-v1.9-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.9.2",
				},
			},
		},
		{
			name: "kubelet-v1.10-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.10.2",
				},
			},
		},
		{
			name: "kubelet-v1.11-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.11.3",
				},
			},
		},
		{
			name: "kubelet-v1.12-aws",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.12.0",
				},
			},
		},
		{
			name: "kubelet-v1.12-vsphere",
			spec: clusterv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: clusterv1alpha1.MachineVersionInfo{
					Kubelet: "1.12.0",
				},
			},
			cloudProviderName: stringPtr("vsphere"),
		},
	}

	defaultCloudProvider := &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil}
	kubeconfig := &clientcmdapi.Config{Clusters: map[string]*clientcmdapi.Cluster{
		"": {Server: "https://server:443", CertificateAuthorityData: []byte(pemCertificate)}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"": {Token: "my-token"}}}
	provider := Provider{}

	for _, test := range tests {
		emtpyProviderConfig := clusterv1alpha1.ProviderConfig{
			Value: &runtime.RawExtension{}}
		test.spec.ProviderConfig = emtpyProviderConfig
		var cloudProvider *fakeCloudConfigProvider
		if test.cloudProviderName != nil {
			cloudProvider = &fakeCloudConfigProvider{name: *test.cloudProviderName, config: "{config:true}", err: nil}
		} else {
			cloudProvider = defaultCloudProvider
		}

		s, err := provider.UserData(test.spec, kubeconfig, cloudProvider, test.clusterDNSIPs)
		if err != nil {
			t.Errorf("error getting userdata: '%v'", err)
		}

		//Check if we can gzip it
		if _, err := convert.GzipString(s); err != nil {
			t.Fatal(err)
		}

		testhelper.CompareOutput(t, test.name, s, *update)
	}
}

func stringPtr(a string) *string {
	return &a
}
