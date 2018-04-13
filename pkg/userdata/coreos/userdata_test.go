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

//TODO: Re-enable once e2e tests verified this stuff works
func testProvider_UserData(t *testing.T) {
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
          "source": "data:,kubeconfig%0A",
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
          "source": "data:,CACert%0A",
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
          "source": "data:,kubeconfig%0A",
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
          "source": "data:,CACert%0A",
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
          "source": "data:,kubeconfig%0A",
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
          "source": "data:,CACert%0A",
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
