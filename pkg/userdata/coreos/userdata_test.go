package coreos

import (
	"encoding/json"
	"net"
	"testing"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/go-test/deep"
	"github.com/pmezard/go-difflib/difflib"
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

	kubeconfig = &clientcmdapi.Config{Clusters: map[string]*clientcmdapi.Cluster{"": &clientcmdapi.Cluster{Server: "https://server:443", CertificateAuthorityData: []byte(pemCertificate)}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"": &clientcmdapi.AuthInfo{Token: "my-token"}}}
)

type fakeCloudConfigProvider struct {
	config string
	name   string
	err    error
}

func (p *fakeCloudConfigProvider) GetCloudConfig(spec machinesv1alpha1.MachineSpec) (config string, name string, err error) {
	return p.config, p.name, p.err
}

func TestProvider_UserData(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		spec             machinesv1alpha1.MachineSpec
		ccProvider       cloud.ConfigProvider
		osConfig         *Config
		providerConfig   *providerconfig.Config
		DNSIPs           []net.IP
		kubernetesCACert string
		resErr           error
		userdata         string
	}{
		{
			name: "docker 1.12.6 disable auto-update aws",
			providerConfig: &providerconfig.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "1.12.6",
					},
					Kubelet: "1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DisableAutoUpdate: true},
			userdata:         docker12DisableAutoUpdateAWS,
		},
		{
			name: "docker 1.12.6 auto-update openstack multiple dns",
			providerConfig: &providerconfig.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "1.12.6",
					},
					Kubelet: "1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10"), net.ParseIP("10.10.10.11"), net.ParseIP("10.10.10.12")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DisableAutoUpdate: false},
			userdata:         docker12AutoUpdateOpenstackMultipleDNS,
		},
		{
			name: "docker 1.12.6 auto-update openstack kubelet v version prefix",
			providerConfig: &providerconfig.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "1.12.6",
					},
					Kubelet: "v1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DisableAutoUpdate: false},
			userdata:         docker12AutoUpdateOpenstack,
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
			spec.ProviderConfig = runtime.RawExtension{Raw: providerConfigRaw}
			p := Provider{}

			userdata, err := p.UserData(spec, kubeconfig, test.ccProvider, test.DNSIPs)
			if diff := deep.Equal(err, test.resErr); diff != nil {
				t.Errorf("expected to get %v instead got: %v", test.resErr, err)
			}
			if err != nil {
				return
			}

			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(test.userdata),
				B:        difflib.SplitLines(userdata),
				FromFile: "Fixture",
				ToFile:   "Current",
				Context:  3,
			}
			diffStr, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Fatal(err)
			}

			if userdata != test.userdata {
				t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
			}
		})
	}
}

var (
	docker12DisableAutoUpdateAWS = `{
  "ignition": {
    "config": {},
    "timeouts": {},
    "version": "2.1.0"
  },
  "networkd": {},
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "ssh-rsa AAABBB",
          "ssh-rsa CCCDDD"
        ]
      }
    ]
  },
  "storage": {
    "files": [
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/bootstrap.kubeconfig",
        "user": {},
        "contents": {
          "source": "data:,apiVersion%3A%20v1%0Aclusters%3A%0A-%20cluster%3A%0A%20%20%20%20certificate-authority-data%3A%20LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVXakNDQTBLZ0F3SUJBZ0lKQUxmUmxXc0k4WVFITUEwR0NTcUdTSWIzRFFFQkJRVUFNSHN4Q3pBSkJnTlYKQkFZVEFsVlRNUXN3Q1FZRFZRUUlFd0pEUVRFV01CUUdBMVVFQnhNTlUyRnVJRVp5WVc1amFYTmpiekVVTUJJRwpBMVVFQ2hNTFFuSmhaR1pwZEhwcGJtTXhFakFRQmdOVkJBTVRDV3h2WTJGc2FHOXpkREVkTUJzR0NTcUdTSWIzCkRRRUpBUllPWW5KaFpFQmtZVzVuWVM1amIyMHdIaGNOTVRRd056RTFNakEwTmpBMVdoY05NVGN3TlRBME1qQTAKTmpBMVdqQjdNUXN3Q1FZRFZRUUdFd0pWVXpFTE1Ba0dBMVVFQ0JNQ1EwRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMEp5WVdSbWFYUjZhVzVqTVJJd0VBWURWUVFERXdsc2IyTmhiR2h2CmMzUXhIVEFiQmdrcWhraUc5dzBCQ1FFV0RtSnlZV1JBWkdGdVoyRXVZMjl0TUlJQklqQU5CZ2txaGtpRzl3MEIKQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBdDVmQWpwNGZUY2VrV1VUZnpzcDBreWloMU9ZYnNHTDBLWDFlUmJTUwpSOE9kMCs5UTYySHlueStHRndNVGI0QS9LVThtc3NvSHZjY2VTQUFid2ZieEZLLytzNTFUb2JxVW5PUlpyT29UClpqa1V5Z2J5WERTSzk5WUJiY1IxUGlwOHZ3TVRtNFhLdUx0Q2lnZUJCZGpqQVFkZ1VPMjhMRU5HbHNNbm1lWWsKSmZPRFZHblZtcjVMdGI5QU5BOElLeVRmc25ISjRpT0NTL1BsUGJVajJxN1lub1ZMcG9zVUJNbGdVYi9DeWtYMwptT29MYjR5SkpReUEvaVNUNlp4aUlFajM2RDR5V1o1bGc3WUpsK1VpaUJRSEdDblBkR3lpcHFWMDZleDBoZVlXCmNhaVc4TFdaU1VROTNqUStXVkNIOGhUN0RRTzFkbXN2VW1YbHEvSmVBbHdRL1FJREFRQUJvNEhnTUlIZE1CMEcKQTFVZERnUVdCQlJjQVJPdGhTNFA0VTd2VGZqQnlDNTY5UjdFNkRDQnJRWURWUjBqQklHbE1JR2lnQlJjQVJPdApoUzRQNFU3dlRmakJ5QzU2OVI3RTZLRi9wSDB3ZXpFTE1Ba0dBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CCk1SWXdGQVlEVlFRSEV3MVRZVzRnUm5KaGJtTnBjMk52TVJRd0VnWURWUVFLRXd0Q2NtRmtabWwwZW1sdVl6RVMKTUJBR0ExVUVBeE1KYkc5allXeG9iM04wTVIwd0d3WUpLb1pJaHZjTkFRa0JGZzVpY21Ga1FHUmhibWRoTG1OdgpiWUlKQUxmUmxXc0k4WVFITUF3R0ExVWRFd1FGTUFNQkFmOHdEUVlKS29aSWh2Y05BUUVGQlFBRGdnRUJBRzZoClU5ZjlzTkgwLzZvQmJHR3kyRVZVMFVnSVRVUUlyRldvOXJGa3JXNWsvWGtEalFtKzNsempUMGlHUjRJeEUvQW8KZVU2c1FodWE3d3JXZUZFbjQ3R0w5OGxuQ3NKZEQ3b1pOaEZtUTk1VGIvTG5EVWpzNVlqOWJyUDBOV3pYZllVNApVSzJabklOSlJjSnBCOGlSQ2FDeEU4RGRjVUYwWHFJRXE2cEEyNzJzbm9MbWlYTE12Tmwza1lFZG0ramU2dm9ECjU4U05WRVVzenR6UXlYbUpFaENwd1ZJMEE2UUNqelhqK3F2cG13M1paSGk4SndYZWk4WlpCTFRTRkJraThaN24Kc0g5QkJIMzgvU3pVbUFONFFIU1B5MWdqcW0wME9BRThOYVlEa2gvYnpFNGQ3bUxHR01XcC9XRTNLUFN1ODJIRgprUGU2WG9TYmlMbS9reGszMlQwPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t%0A%20%20%20%20server%3A%20https%3A%2F%2Fserver%3A443%0A%20%20name%3A%20%22%22%0Acontexts%3A%20%5B%5D%0Acurrent-context%3A%20%22%22%0Akind%3A%20Config%0Apreferences%3A%20%7B%7D%0Ausers%3A%0A-%20name%3A%20%22%22%0A%20%20user%3A%0A%20%20%20%20token%3A%20my-token%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Baws-config%3Atrue%7D%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/ca.crt",
        "user": {},
        "contents": {
          "source": "data:,-----BEGIN%20CERTIFICATE-----%0AMIIEWjCCA0KgAwIBAgIJALfRlWsI8YQHMA0GCSqGSIb3DQEBBQUAMHsxCzAJBgNV%0ABAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEUMBIG%0AA1UEChMLQnJhZGZpdHppbmMxEjAQBgNVBAMTCWxvY2FsaG9zdDEdMBsGCSqGSIb3%0ADQEJARYOYnJhZEBkYW5nYS5jb20wHhcNMTQwNzE1MjA0NjA1WhcNMTcwNTA0MjA0%0ANjA1WjB7MQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAUBgNVBAcTDVNhbiBG%0AcmFuY2lzY28xFDASBgNVBAoTC0JyYWRmaXR6aW5jMRIwEAYDVQQDEwlsb2NhbGhv%0Ac3QxHTAbBgkqhkiG9w0BCQEWDmJyYWRAZGFuZ2EuY29tMIIBIjANBgkqhkiG9w0B%0AAQEFAAOCAQ8AMIIBCgKCAQEAt5fAjp4fTcekWUTfzsp0kyih1OYbsGL0KX1eRbSS%0AR8Od0%2B9Q62Hyny%2BGFwMTb4A%2FKU8mssoHvcceSAAbwfbxFK%2F%2Bs51TobqUnORZrOoT%0AZjkUygbyXDSK99YBbcR1Pip8vwMTm4XKuLtCigeBBdjjAQdgUO28LENGlsMnmeYk%0AJfODVGnVmr5Ltb9ANA8IKyTfsnHJ4iOCS%2FPlPbUj2q7YnoVLposUBMlgUb%2FCykX3%0AmOoLb4yJJQyA%2FiST6ZxiIEj36D4yWZ5lg7YJl%2BUiiBQHGCnPdGyipqV06ex0heYW%0AcaiW8LWZSUQ93jQ%2BWVCH8hT7DQO1dmsvUmXlq%2FJeAlwQ%2FQIDAQABo4HgMIHdMB0G%0AA1UdDgQWBBRcAROthS4P4U7vTfjByC569R7E6DCBrQYDVR0jBIGlMIGigBRcAROt%0AhS4P4U7vTfjByC569R7E6KF%2FpH0wezELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNB%0AMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMRQwEgYDVQQKEwtCcmFkZml0emluYzES%0AMBAGA1UEAxMJbG9jYWxob3N0MR0wGwYJKoZIhvcNAQkBFg5icmFkQGRhbmdhLmNv%0AbYIJALfRlWsI8YQHMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAG6h%0AU9f9sNH0%2F6oBbGGy2EVU0UgITUQIrFWo9rFkrW5k%2FXkDjQm%2B3lzjT0iGR4IxE%2FAo%0AeU6sQhua7wrWeFEn47GL98lnCsJdD7oZNhFmQ95Tb%2FLnDUjs5Yj9brP0NWzXfYU4%0AUK2ZnINJRcJpB8iRCaCxE8DdcUF0XqIEq6pA272snoLmiXLMvNl3kYEdm%2Bje6voD%0A58SNVEUsztzQyXmJEhCpwVI0A6QCjzXj%2Bqvpmw3ZZHi8JwXei8ZZBLTSFBki8Z7n%0AsH9BBH38%2FSzUmAN4QHSPy1gjqm00OAE8NaYDkh%2FbzE4d7mLGGMWp%2FWE3KPSu82HF%0AkPe6XoSbiLm%2Fkxk32T0%3D%0A-----END%20CERTIFICATE-----%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/hostname",
        "user": {},
        "contents": {
          "source": "data:,node1",
          "verification": {}
        },
        "mode": 384
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "mask": true,
        "name": "update-engine.service"
      },
      {
        "mask": true,
        "name": "locksmithd.service"
      },
      {
        "enabled": true,
        "name": "docker.service"
      },
      {
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=aws \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention \\\n  --read-only-port 0 \\\n  --authorization-mode=Webhook \\\n  --anonymous-auth=false \\\n  --client-ca-file=/etc/kubernetes/ca.crt\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
        "dropins": [
          {
            "contents": "[Unit]\nRequires=docker.service\nAfter=docker.service\n",
            "name": "40-docker.conf"
          }
        ],
        "enabled": true,
        "name": "kubelet.service"
      }
    ]
  }
}`

	docker12AutoUpdateOpenstack = `{
  "ignition": {
    "config": {},
    "timeouts": {},
    "version": "2.1.0"
  },
  "networkd": {},
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "ssh-rsa AAABBB",
          "ssh-rsa CCCDDD"
        ]
      }
    ]
  },
  "storage": {
    "files": [
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/bootstrap.kubeconfig",
        "user": {},
        "contents": {
          "source": "data:,apiVersion%3A%20v1%0Aclusters%3A%0A-%20cluster%3A%0A%20%20%20%20certificate-authority-data%3A%20LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVXakNDQTBLZ0F3SUJBZ0lKQUxmUmxXc0k4WVFITUEwR0NTcUdTSWIzRFFFQkJRVUFNSHN4Q3pBSkJnTlYKQkFZVEFsVlRNUXN3Q1FZRFZRUUlFd0pEUVRFV01CUUdBMVVFQnhNTlUyRnVJRVp5WVc1amFYTmpiekVVTUJJRwpBMVVFQ2hNTFFuSmhaR1pwZEhwcGJtTXhFakFRQmdOVkJBTVRDV3h2WTJGc2FHOXpkREVkTUJzR0NTcUdTSWIzCkRRRUpBUllPWW5KaFpFQmtZVzVuWVM1amIyMHdIaGNOTVRRd056RTFNakEwTmpBMVdoY05NVGN3TlRBME1qQTAKTmpBMVdqQjdNUXN3Q1FZRFZRUUdFd0pWVXpFTE1Ba0dBMVVFQ0JNQ1EwRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMEp5WVdSbWFYUjZhVzVqTVJJd0VBWURWUVFERXdsc2IyTmhiR2h2CmMzUXhIVEFiQmdrcWhraUc5dzBCQ1FFV0RtSnlZV1JBWkdGdVoyRXVZMjl0TUlJQklqQU5CZ2txaGtpRzl3MEIKQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBdDVmQWpwNGZUY2VrV1VUZnpzcDBreWloMU9ZYnNHTDBLWDFlUmJTUwpSOE9kMCs5UTYySHlueStHRndNVGI0QS9LVThtc3NvSHZjY2VTQUFid2ZieEZLLytzNTFUb2JxVW5PUlpyT29UClpqa1V5Z2J5WERTSzk5WUJiY1IxUGlwOHZ3TVRtNFhLdUx0Q2lnZUJCZGpqQVFkZ1VPMjhMRU5HbHNNbm1lWWsKSmZPRFZHblZtcjVMdGI5QU5BOElLeVRmc25ISjRpT0NTL1BsUGJVajJxN1lub1ZMcG9zVUJNbGdVYi9DeWtYMwptT29MYjR5SkpReUEvaVNUNlp4aUlFajM2RDR5V1o1bGc3WUpsK1VpaUJRSEdDblBkR3lpcHFWMDZleDBoZVlXCmNhaVc4TFdaU1VROTNqUStXVkNIOGhUN0RRTzFkbXN2VW1YbHEvSmVBbHdRL1FJREFRQUJvNEhnTUlIZE1CMEcKQTFVZERnUVdCQlJjQVJPdGhTNFA0VTd2VGZqQnlDNTY5UjdFNkRDQnJRWURWUjBqQklHbE1JR2lnQlJjQVJPdApoUzRQNFU3dlRmakJ5QzU2OVI3RTZLRi9wSDB3ZXpFTE1Ba0dBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CCk1SWXdGQVlEVlFRSEV3MVRZVzRnUm5KaGJtTnBjMk52TVJRd0VnWURWUVFLRXd0Q2NtRmtabWwwZW1sdVl6RVMKTUJBR0ExVUVBeE1KYkc5allXeG9iM04wTVIwd0d3WUpLb1pJaHZjTkFRa0JGZzVpY21Ga1FHUmhibWRoTG1OdgpiWUlKQUxmUmxXc0k4WVFITUF3R0ExVWRFd1FGTUFNQkFmOHdEUVlKS29aSWh2Y05BUUVGQlFBRGdnRUJBRzZoClU5ZjlzTkgwLzZvQmJHR3kyRVZVMFVnSVRVUUlyRldvOXJGa3JXNWsvWGtEalFtKzNsempUMGlHUjRJeEUvQW8KZVU2c1FodWE3d3JXZUZFbjQ3R0w5OGxuQ3NKZEQ3b1pOaEZtUTk1VGIvTG5EVWpzNVlqOWJyUDBOV3pYZllVNApVSzJabklOSlJjSnBCOGlSQ2FDeEU4RGRjVUYwWHFJRXE2cEEyNzJzbm9MbWlYTE12Tmwza1lFZG0ramU2dm9ECjU4U05WRVVzenR6UXlYbUpFaENwd1ZJMEE2UUNqelhqK3F2cG13M1paSGk4SndYZWk4WlpCTFRTRkJraThaN24Kc0g5QkJIMzgvU3pVbUFONFFIU1B5MWdqcW0wME9BRThOYVlEa2gvYnpFNGQ3bUxHR01XcC9XRTNLUFN1ODJIRgprUGU2WG9TYmlMbS9reGszMlQwPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t%0A%20%20%20%20server%3A%20https%3A%2F%2Fserver%3A443%0A%20%20name%3A%20%22%22%0Acontexts%3A%20%5B%5D%0Acurrent-context%3A%20%22%22%0Akind%3A%20Config%0Apreferences%3A%20%7B%7D%0Ausers%3A%0A-%20name%3A%20%22%22%0A%20%20user%3A%0A%20%20%20%20token%3A%20my-token%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Bopenstack-config%3Atrue%7D%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/ca.crt",
        "user": {},
        "contents": {
          "source": "data:,-----BEGIN%20CERTIFICATE-----%0AMIIEWjCCA0KgAwIBAgIJALfRlWsI8YQHMA0GCSqGSIb3DQEBBQUAMHsxCzAJBgNV%0ABAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEUMBIG%0AA1UEChMLQnJhZGZpdHppbmMxEjAQBgNVBAMTCWxvY2FsaG9zdDEdMBsGCSqGSIb3%0ADQEJARYOYnJhZEBkYW5nYS5jb20wHhcNMTQwNzE1MjA0NjA1WhcNMTcwNTA0MjA0%0ANjA1WjB7MQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAUBgNVBAcTDVNhbiBG%0AcmFuY2lzY28xFDASBgNVBAoTC0JyYWRmaXR6aW5jMRIwEAYDVQQDEwlsb2NhbGhv%0Ac3QxHTAbBgkqhkiG9w0BCQEWDmJyYWRAZGFuZ2EuY29tMIIBIjANBgkqhkiG9w0B%0AAQEFAAOCAQ8AMIIBCgKCAQEAt5fAjp4fTcekWUTfzsp0kyih1OYbsGL0KX1eRbSS%0AR8Od0%2B9Q62Hyny%2BGFwMTb4A%2FKU8mssoHvcceSAAbwfbxFK%2F%2Bs51TobqUnORZrOoT%0AZjkUygbyXDSK99YBbcR1Pip8vwMTm4XKuLtCigeBBdjjAQdgUO28LENGlsMnmeYk%0AJfODVGnVmr5Ltb9ANA8IKyTfsnHJ4iOCS%2FPlPbUj2q7YnoVLposUBMlgUb%2FCykX3%0AmOoLb4yJJQyA%2FiST6ZxiIEj36D4yWZ5lg7YJl%2BUiiBQHGCnPdGyipqV06ex0heYW%0AcaiW8LWZSUQ93jQ%2BWVCH8hT7DQO1dmsvUmXlq%2FJeAlwQ%2FQIDAQABo4HgMIHdMB0G%0AA1UdDgQWBBRcAROthS4P4U7vTfjByC569R7E6DCBrQYDVR0jBIGlMIGigBRcAROt%0AhS4P4U7vTfjByC569R7E6KF%2FpH0wezELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNB%0AMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMRQwEgYDVQQKEwtCcmFkZml0emluYzES%0AMBAGA1UEAxMJbG9jYWxob3N0MR0wGwYJKoZIhvcNAQkBFg5icmFkQGRhbmdhLmNv%0AbYIJALfRlWsI8YQHMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAG6h%0AU9f9sNH0%2F6oBbGGy2EVU0UgITUQIrFWo9rFkrW5k%2FXkDjQm%2B3lzjT0iGR4IxE%2FAo%0AeU6sQhua7wrWeFEn47GL98lnCsJdD7oZNhFmQ95Tb%2FLnDUjs5Yj9brP0NWzXfYU4%0AUK2ZnINJRcJpB8iRCaCxE8DdcUF0XqIEq6pA272snoLmiXLMvNl3kYEdm%2Bje6voD%0A58SNVEUsztzQyXmJEhCpwVI0A6QCjzXj%2Bqvpmw3ZZHi8JwXei8ZZBLTSFBki8Z7n%0AsH9BBH38%2FSzUmAN4QHSPy1gjqm00OAE8NaYDkh%2FbzE4d7mLGGMWp%2FWE3KPSu82HF%0AkPe6XoSbiLm%2Fkxk32T0%3D%0A-----END%20CERTIFICATE-----%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/hostname",
        "user": {},
        "contents": {
          "source": "data:,node1",
          "verification": {}
        },
        "mode": 384
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "enabled": true,
        "name": "docker.service"
      },
      {
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=openstack \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention \\\n  --read-only-port 0 \\\n  --authorization-mode=Webhook \\\n  --anonymous-auth=false \\\n  --client-ca-file=/etc/kubernetes/ca.crt\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
        "dropins": [
          {
            "contents": "[Unit]\nRequires=docker.service\nAfter=docker.service\n",
            "name": "40-docker.conf"
          }
        ],
        "enabled": true,
        "name": "kubelet.service"
      }
    ]
  }
}`

	docker12AutoUpdateOpenstackMultipleDNS = `{
  "ignition": {
    "config": {},
    "timeouts": {},
    "version": "2.1.0"
  },
  "networkd": {},
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "ssh-rsa AAABBB",
          "ssh-rsa CCCDDD"
        ]
      }
    ]
  },
  "storage": {
    "files": [
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/bootstrap.kubeconfig",
        "user": {},
        "contents": {
          "source": "data:,apiVersion%3A%20v1%0Aclusters%3A%0A-%20cluster%3A%0A%20%20%20%20certificate-authority-data%3A%20LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVXakNDQTBLZ0F3SUJBZ0lKQUxmUmxXc0k4WVFITUEwR0NTcUdTSWIzRFFFQkJRVUFNSHN4Q3pBSkJnTlYKQkFZVEFsVlRNUXN3Q1FZRFZRUUlFd0pEUVRFV01CUUdBMVVFQnhNTlUyRnVJRVp5WVc1amFYTmpiekVVTUJJRwpBMVVFQ2hNTFFuSmhaR1pwZEhwcGJtTXhFakFRQmdOVkJBTVRDV3h2WTJGc2FHOXpkREVkTUJzR0NTcUdTSWIzCkRRRUpBUllPWW5KaFpFQmtZVzVuWVM1amIyMHdIaGNOTVRRd056RTFNakEwTmpBMVdoY05NVGN3TlRBME1qQTAKTmpBMVdqQjdNUXN3Q1FZRFZRUUdFd0pWVXpFTE1Ba0dBMVVFQ0JNQ1EwRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMEp5WVdSbWFYUjZhVzVqTVJJd0VBWURWUVFERXdsc2IyTmhiR2h2CmMzUXhIVEFiQmdrcWhraUc5dzBCQ1FFV0RtSnlZV1JBWkdGdVoyRXVZMjl0TUlJQklqQU5CZ2txaGtpRzl3MEIKQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBdDVmQWpwNGZUY2VrV1VUZnpzcDBreWloMU9ZYnNHTDBLWDFlUmJTUwpSOE9kMCs5UTYySHlueStHRndNVGI0QS9LVThtc3NvSHZjY2VTQUFid2ZieEZLLytzNTFUb2JxVW5PUlpyT29UClpqa1V5Z2J5WERTSzk5WUJiY1IxUGlwOHZ3TVRtNFhLdUx0Q2lnZUJCZGpqQVFkZ1VPMjhMRU5HbHNNbm1lWWsKSmZPRFZHblZtcjVMdGI5QU5BOElLeVRmc25ISjRpT0NTL1BsUGJVajJxN1lub1ZMcG9zVUJNbGdVYi9DeWtYMwptT29MYjR5SkpReUEvaVNUNlp4aUlFajM2RDR5V1o1bGc3WUpsK1VpaUJRSEdDblBkR3lpcHFWMDZleDBoZVlXCmNhaVc4TFdaU1VROTNqUStXVkNIOGhUN0RRTzFkbXN2VW1YbHEvSmVBbHdRL1FJREFRQUJvNEhnTUlIZE1CMEcKQTFVZERnUVdCQlJjQVJPdGhTNFA0VTd2VGZqQnlDNTY5UjdFNkRDQnJRWURWUjBqQklHbE1JR2lnQlJjQVJPdApoUzRQNFU3dlRmakJ5QzU2OVI3RTZLRi9wSDB3ZXpFTE1Ba0dBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CCk1SWXdGQVlEVlFRSEV3MVRZVzRnUm5KaGJtTnBjMk52TVJRd0VnWURWUVFLRXd0Q2NtRmtabWwwZW1sdVl6RVMKTUJBR0ExVUVBeE1KYkc5allXeG9iM04wTVIwd0d3WUpLb1pJaHZjTkFRa0JGZzVpY21Ga1FHUmhibWRoTG1OdgpiWUlKQUxmUmxXc0k4WVFITUF3R0ExVWRFd1FGTUFNQkFmOHdEUVlKS29aSWh2Y05BUUVGQlFBRGdnRUJBRzZoClU5ZjlzTkgwLzZvQmJHR3kyRVZVMFVnSVRVUUlyRldvOXJGa3JXNWsvWGtEalFtKzNsempUMGlHUjRJeEUvQW8KZVU2c1FodWE3d3JXZUZFbjQ3R0w5OGxuQ3NKZEQ3b1pOaEZtUTk1VGIvTG5EVWpzNVlqOWJyUDBOV3pYZllVNApVSzJabklOSlJjSnBCOGlSQ2FDeEU4RGRjVUYwWHFJRXE2cEEyNzJzbm9MbWlYTE12Tmwza1lFZG0ramU2dm9ECjU4U05WRVVzenR6UXlYbUpFaENwd1ZJMEE2UUNqelhqK3F2cG13M1paSGk4SndYZWk4WlpCTFRTRkJraThaN24Kc0g5QkJIMzgvU3pVbUFONFFIU1B5MWdqcW0wME9BRThOYVlEa2gvYnpFNGQ3bUxHR01XcC9XRTNLUFN1ODJIRgprUGU2WG9TYmlMbS9reGszMlQwPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t%0A%20%20%20%20server%3A%20https%3A%2F%2Fserver%3A443%0A%20%20name%3A%20%22%22%0Acontexts%3A%20%5B%5D%0Acurrent-context%3A%20%22%22%0Akind%3A%20Config%0Apreferences%3A%20%7B%7D%0Ausers%3A%0A-%20name%3A%20%22%22%0A%20%20user%3A%0A%20%20%20%20token%3A%20my-token%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Bopenstack-config%3Atrue%7D%0A",
          "verification": {}
        },
        "mode": 256
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/ca.crt",
        "user": {},
        "contents": {
          "source": "data:,-----BEGIN%20CERTIFICATE-----%0AMIIEWjCCA0KgAwIBAgIJALfRlWsI8YQHMA0GCSqGSIb3DQEBBQUAMHsxCzAJBgNV%0ABAYTAlVTMQswCQYDVQQIEwJDQTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEUMBIG%0AA1UEChMLQnJhZGZpdHppbmMxEjAQBgNVBAMTCWxvY2FsaG9zdDEdMBsGCSqGSIb3%0ADQEJARYOYnJhZEBkYW5nYS5jb20wHhcNMTQwNzE1MjA0NjA1WhcNMTcwNTA0MjA0%0ANjA1WjB7MQswCQYDVQQGEwJVUzELMAkGA1UECBMCQ0ExFjAUBgNVBAcTDVNhbiBG%0AcmFuY2lzY28xFDASBgNVBAoTC0JyYWRmaXR6aW5jMRIwEAYDVQQDEwlsb2NhbGhv%0Ac3QxHTAbBgkqhkiG9w0BCQEWDmJyYWRAZGFuZ2EuY29tMIIBIjANBgkqhkiG9w0B%0AAQEFAAOCAQ8AMIIBCgKCAQEAt5fAjp4fTcekWUTfzsp0kyih1OYbsGL0KX1eRbSS%0AR8Od0%2B9Q62Hyny%2BGFwMTb4A%2FKU8mssoHvcceSAAbwfbxFK%2F%2Bs51TobqUnORZrOoT%0AZjkUygbyXDSK99YBbcR1Pip8vwMTm4XKuLtCigeBBdjjAQdgUO28LENGlsMnmeYk%0AJfODVGnVmr5Ltb9ANA8IKyTfsnHJ4iOCS%2FPlPbUj2q7YnoVLposUBMlgUb%2FCykX3%0AmOoLb4yJJQyA%2FiST6ZxiIEj36D4yWZ5lg7YJl%2BUiiBQHGCnPdGyipqV06ex0heYW%0AcaiW8LWZSUQ93jQ%2BWVCH8hT7DQO1dmsvUmXlq%2FJeAlwQ%2FQIDAQABo4HgMIHdMB0G%0AA1UdDgQWBBRcAROthS4P4U7vTfjByC569R7E6DCBrQYDVR0jBIGlMIGigBRcAROt%0AhS4P4U7vTfjByC569R7E6KF%2FpH0wezELMAkGA1UEBhMCVVMxCzAJBgNVBAgTAkNB%0AMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMRQwEgYDVQQKEwtCcmFkZml0emluYzES%0AMBAGA1UEAxMJbG9jYWxob3N0MR0wGwYJKoZIhvcNAQkBFg5icmFkQGRhbmdhLmNv%0AbYIJALfRlWsI8YQHMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAG6h%0AU9f9sNH0%2F6oBbGGy2EVU0UgITUQIrFWo9rFkrW5k%2FXkDjQm%2B3lzjT0iGR4IxE%2FAo%0AeU6sQhua7wrWeFEn47GL98lnCsJdD7oZNhFmQ95Tb%2FLnDUjs5Yj9brP0NWzXfYU4%0AUK2ZnINJRcJpB8iRCaCxE8DdcUF0XqIEq6pA272snoLmiXLMvNl3kYEdm%2Bje6voD%0A58SNVEUsztzQyXmJEhCpwVI0A6QCjzXj%2Bqvpmw3ZZHi8JwXei8ZZBLTSFBki8Z7n%0AsH9BBH38%2FSzUmAN4QHSPy1gjqm00OAE8NaYDkh%2FbzE4d7mLGGMWp%2FWE3KPSu82HF%0AkPe6XoSbiLm%2Fkxk32T0%3D%0A-----END%20CERTIFICATE-----%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        },
        "mode": 420
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/hostname",
        "user": {},
        "contents": {
          "source": "data:,node1",
          "verification": {}
        },
        "mode": 384
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "enabled": true,
        "name": "docker.service"
      },
      {
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10,10.10.10.11,10.10.10.12 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=openstack \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention \\\n  --read-only-port 0 \\\n  --authorization-mode=Webhook \\\n  --anonymous-auth=false \\\n  --client-ca-file=/etc/kubernetes/ca.crt\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
        "dropins": [
          {
            "contents": "[Unit]\nRequires=docker.service\nAfter=docker.service\n",
            "name": "40-docker.conf"
          }
        ],
        "enabled": true,
        "name": "kubelet.service"
      }
    ]
  }
}`
)
