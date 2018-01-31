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

	"github.com/go-test/deep"
	"github.com/pmezard/go-difflib/difflib"
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
		name           string
		spec           machinesv1alpha1.MachineSpec
		kubeconfig     string
		ccProvider     cloud.ConfigProvider
		osConfig       *config
		providerConfig *providerconfig.Config
		DNSIPs         []net.IP
		resErr         error
		userdata       string
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
			ccProvider: &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil},
			kubeconfig: "kubeconfig",
			DNSIPs:     []net.IP{net.ParseIP("10.10.10.10")},
			resErr:     nil,
			osConfig:   &config{DisableAutoUpdate: true},
			userdata:   docker12DisableAutoUpdateAWS,
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
			ccProvider: &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			kubeconfig: "kubeconfig",
			DNSIPs:     []net.IP{net.ParseIP("10.10.10.10"), net.ParseIP("10.10.10.11"), net.ParseIP("10.10.10.12")},
			resErr:     nil,
			osConfig:   &config{DisableAutoUpdate: false},
			userdata:   docker12AutoUpdateOpenstackMultipleDNS,
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
			ccProvider: &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			kubeconfig: "kubeconfig",
			DNSIPs:     []net.IP{net.ParseIP("10.10.10.10")},
			resErr:     nil,
			osConfig:   &config{DisableAutoUpdate: false},
			userdata:   docker12AutoUpdateOpenstack,
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

			userdata, err := p.UserData(spec, test.kubeconfig, test.ccProvider, test.DNSIPs)
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
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Baws-config%3Atrue%7D%0A",
          "verification": {}
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        }
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
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=aws \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
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
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Bopenstack-config%3Atrue%7D%0A",
          "verification": {}
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        }
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
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=openstack \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
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
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/kubernetes/cloud-config",
        "user": {},
        "contents": {
          "source": "data:,%7Bopenstack-config%3Atrue%7D%0A",
          "verification": {}
        }
      },
      {
        "filesystem": "root",
        "group": {},
        "path": "/etc/coreos/docker-1.12",
        "user": {},
        "contents": {
          "source": "data:,yes%0A",
          "verification": {}
        }
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
        "contents": "[Unit]\nDescription=Kubernetes Kubelet\nRequires=docker.service\nAfter=docker.service\n[Service]\nTimeoutStartSec=5min\nEnvironment=KUBELET_IMAGE_TAG=v1.9.2_coreos.0\nEnvironment=\"RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \\\n  --volume=resolv,kind=host,source=/etc/resolv.conf \\\n  --mount volume=resolv,target=/etc/resolv.conf \\\n  --volume cni-bin,kind=host,source=/opt/cni/bin \\\n  --mount volume=cni-bin,target=/opt/cni/bin \\\n  --volume cni-conf,kind=host,source=/etc/cni/net.d \\\n  --mount volume=cni-conf,target=/etc/cni/net.d \\\n  --volume etc-kubernetes,kind=host,source=/etc/kubernetes \\\n  --mount volume=etc-kubernetes,target=/etc/kubernetes \\\n  --volume var-log,kind=host,source=/var/log \\\n  --mount volume=var-log,target=/var/log\"\nExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests\nExecStartPre=/bin/mkdir -p /etc/cni/net.d\nExecStartPre=/bin/mkdir -p /opt/cni/bin\nExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid\nExecStart=/usr/lib/coreos/kubelet-wrapper \\\n  --container-runtime=docker \\\n  --allow-privileged=true \\\n  --cni-bin-dir=/opt/cni/bin \\\n  --cni-conf-dir=/etc/cni/net.d \\\n  --cluster-dns=10.10.10.10,10.10.10.11,10.10.10.12 \\\n  --cluster-domain=cluster.local \\\n  --network-plugin=cni \\\n  --cloud-provider=openstack \\\n  --cloud-config=/etc/kubernetes/cloud-config \\\n  --cert-dir=/etc/kubernetes/ \\\n  --pod-manifest-path=/etc/kubernetes/manifests \\\n  --resolv-conf=/etc/resolv.conf \\\n  --rotate-certificates=true \\\n  --kubeconfig=/etc/kubernetes/kubeconfig \\\n  --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \\\n  --lock-file=/var/run/lock/kubelet.lock \\\n  --exit-on-lock-contention\nExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid\nRestart=always\nRestartSec=10\n[Install]\nWantedBy=multi-user.target\n",
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
