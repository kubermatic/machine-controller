package ubuntu

import (
	"encoding/json"
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

		resErr   error
		userdata string
	}{
		{
			name: "docker 1.13 dist-upgrade-on-boot aws",
			providerConfig: &providerconfig.Config{
				CloudProvider: "aws",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "1.13.1",
					},
					Kubelet: "1.9.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil},
			kubeconfig: "kubeconfig",
			resErr:     nil,
			osConfig:   &config{DistUpgradeOnBoot: true},
			userdata:   docker12DistupgradeAWS,
		},
		{
			name: "cri-o 1.9 digitalocean",
			providerConfig: &providerconfig.Config{
				CloudProvider: "digitalocean",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "cri-o",
						Version: "1.9",
					},
					Kubelet: "1.9.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{name: "", config: "", err: nil},
			kubeconfig: "kubeconfig",
			resErr:     nil,
			osConfig:   &config{DistUpgradeOnBoot: false},
			userdata:   CRIO19Digitalocean,
		},
		{
			name: "docker 17.03 openstack",
			providerConfig: &providerconfig.Config{
				CloudProvider: "openstack",
				SSHPublicKeys: []string{"ssh-rsa AAABBB", "ssh-rsa CCCDDD"},
			},
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "17.03.2",
					},
					Kubelet: "1.9.2",
				},
			},
			ccProvider: &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			kubeconfig: "kubeconfig",
			resErr:     nil,
			osConfig:   &config{DistUpgradeOnBoot: true},
			userdata:   docker1703DistupgradeOpenstack,
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

			userdata, err := p.UserData(spec, test.kubeconfig, test.ccProvider)
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
	docker12DistupgradeAWS = `#cloud-config
hostname: node1

package_update: true
package_upgrade: true
package_reboot_if_required: true

ssh_authorized_keys:
- "ssh-rsa AAABBB"
- "ssh-rsa CCCDDD"

write_files:
- path: "/etc/kubernetes/cloud-config"
  content: |
    {aws-config:true}

- path: "/etc/kubernetes/bootstrap.kubeconfig"
  content: |
    kubeconfig

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    if [ ! -f /opt/bin/kubelet ]; then
      curl -L -o /opt/bin/kubelet https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/kubelet
      chmod +x /opt/bin/kubelet
    fi
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://github.com/containernetworking/plugins/releases/download/v0.6.0/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network.target
    After=network.target

    [Service]
    Restart=always
    RestartSec=10
    StartLimitInterval=600
    StartLimitBurst=50
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/etc/kubernetes/download.sh
    ExecStart=/opt/bin/kubelet \
      --container-runtime=docker \
      --allow-privileged=true \
      --cni-bin-dir=/opt/cni/bin \
      --cni-conf-dir=/etc/cni/net.d \
      --cluster-dns=10.10.10.10 \
      --cluster-domain=cluster.local \
      --network-plugin=cni \
      --cloud-provider=aws \
      --cloud-config=/etc/kubernetes/cloud-config \
      --cert-dir=/etc/kubernetes/ \
      --pod-manifest-path=/etc/kubernetes/manifests \
      --resolv-conf=/etc/resolv.conf \
      --rotate-certificates=true \
      --kubeconfig=/etc/kubernetes/kubeconfig \
      --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \
      --lock-file=/var/run/lock/kubelet.lock \
      --exit-on-lock-contention

    [Install]
    WantedBy=multi-user.target

runcmd:
- systemctl enable kubelet
- systemctl start kubelet

apt:
  sources:
    docker:
      source: deb [arch=amd64] https://download.docker.com/linux/ubuntu $RELEASE stable
      key: |
        -----BEGIN PGP PUBLIC KEY BLOCK-----

        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
        YT90qFF93M3v01BbxP+EIY2/9tiIPbrd
        =0YYh
        -----END PGP PUBLIC KEY BLOCK-----

# install dependencies for cloud-init via bootcmd...
bootcmd:
- "sudo apt-get update && sudo apt-get install -y software-properties-common gdisk eatmydata"

packages:
- "curl"
- "ca-certificates"
- "ceph-common"
- "cifs-utils"
- "conntrack"
- "e2fsprogs"
- "ebtables"
- "ethtool"
- "git"
- "glusterfs-client"
- "iptables"
- "jq"
- "kmod"
- "openssh-client"
- "nfs-common"
- "socat"
- "util-linux"
- ["docker.io", "1.13.1-0ubuntu1~16.04.2"]
`

	CRIO19Digitalocean = `#cloud-config
hostname: node1

package_update: true

ssh_authorized_keys:
- "ssh-rsa AAABBB"
- "ssh-rsa CCCDDD"

write_files:
- path: "/etc/kubernetes/cloud-config"
  content: |
    

- path: "/etc/kubernetes/bootstrap.kubeconfig"
  content: |
    kubeconfig

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    if [ ! -f /opt/bin/kubelet ]; then
      curl -L -o /opt/bin/kubelet https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/kubelet
      chmod +x /opt/bin/kubelet
    fi
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://github.com/containernetworking/plugins/releases/download/v0.6.0/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network.target
    After=network.target

    [Service]
    Restart=always
    RestartSec=10
    StartLimitInterval=600
    StartLimitBurst=50
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/etc/kubernetes/download.sh
    ExecStart=/opt/bin/kubelet \
      --container-runtime=remote \
      --container-runtime-endpoint=unix:///var/run/crio/crio.sock \
      --cgroup-driver="systemd" \
      --allow-privileged=true \
      --cni-bin-dir=/opt/cni/bin \
      --cni-conf-dir=/etc/cni/net.d \
      --cluster-dns=10.10.10.10 \
      --cluster-domain=cluster.local \
      --network-plugin=cni \
      --cert-dir=/etc/kubernetes/ \
      --pod-manifest-path=/etc/kubernetes/manifests \
      --resolv-conf=/etc/resolv.conf \
      --rotate-certificates=true \
      --kubeconfig=/etc/kubernetes/kubeconfig \
      --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \
      --lock-file=/var/run/lock/kubelet.lock \
      --exit-on-lock-contention

    [Install]
    WantedBy=multi-user.target

- path: "/etc/sysconfig/crio-network"
  content: |
    CRIO_NETWORK_OPTIONS="--registry=docker.io"

runcmd:
- systemctl enable crio
- systemctl start crio
- systemctl enable kubelet
- systemctl start kubelet

apt:
  sources:
    cri-o:
      source: "ppa:projectatomic/ppa"

# install dependencies for cloud-init via bootcmd...
bootcmd:
- "sudo apt-get update && sudo apt-get install -y software-properties-common gdisk eatmydata"

packages:
- "curl"
- "ca-certificates"
- "ceph-common"
- "cifs-utils"
- "conntrack"
- "e2fsprogs"
- "ebtables"
- "ethtool"
- "git"
- "glusterfs-client"
- "iptables"
- "jq"
- "kmod"
- "openssh-client"
- "nfs-common"
- "socat"
- "util-linux"
- ["cri-o", "1.9.0-1~ubuntu16.04.2~ppa1"]
`

	docker1703DistupgradeOpenstack = `#cloud-config
hostname: node1

package_update: true
package_upgrade: true
package_reboot_if_required: true

ssh_authorized_keys:
- "ssh-rsa AAABBB"
- "ssh-rsa CCCDDD"

write_files:
- path: "/etc/kubernetes/cloud-config"
  content: |
    {openstack-config:true}

- path: "/etc/kubernetes/bootstrap.kubeconfig"
  content: |
    kubeconfig

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    if [ ! -f /opt/bin/kubelet ]; then
      curl -L -o /opt/bin/kubelet https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/kubelet
      chmod +x /opt/bin/kubelet
    fi
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://github.com/containernetworking/plugins/releases/download/v0.6.0/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network.target
    After=network.target

    [Service]
    Restart=always
    RestartSec=10
    StartLimitInterval=600
    StartLimitBurst=50
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/etc/kubernetes/download.sh
    ExecStart=/opt/bin/kubelet \
      --container-runtime=docker \
      --allow-privileged=true \
      --cni-bin-dir=/opt/cni/bin \
      --cni-conf-dir=/etc/cni/net.d \
      --cluster-dns=10.10.10.10 \
      --cluster-domain=cluster.local \
      --network-plugin=cni \
      --cloud-provider=openstack \
      --cloud-config=/etc/kubernetes/cloud-config \
      --cert-dir=/etc/kubernetes/ \
      --pod-manifest-path=/etc/kubernetes/manifests \
      --resolv-conf=/etc/resolv.conf \
      --rotate-certificates=true \
      --kubeconfig=/etc/kubernetes/kubeconfig \
      --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \
      --lock-file=/var/run/lock/kubelet.lock \
      --exit-on-lock-contention

    [Install]
    WantedBy=multi-user.target

runcmd:
- systemctl enable kubelet
- systemctl start kubelet

apt:
  sources:
    docker:
      source: deb [arch=amd64] https://download.docker.com/linux/ubuntu $RELEASE stable
      key: |
        -----BEGIN PGP PUBLIC KEY BLOCK-----

        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
        YT90qFF93M3v01BbxP+EIY2/9tiIPbrd
        =0YYh
        -----END PGP PUBLIC KEY BLOCK-----

# install dependencies for cloud-init via bootcmd...
bootcmd:
- "sudo apt-get update && sudo apt-get install -y software-properties-common gdisk eatmydata"

packages:
- "curl"
- "ca-certificates"
- "ceph-common"
- "cifs-utils"
- "conntrack"
- "e2fsprogs"
- "ebtables"
- "ethtool"
- "git"
- "glusterfs-client"
- "iptables"
- "jq"
- "kmod"
- "openssh-client"
- "nfs-common"
- "socat"
- "util-linux"
- ["docker-ce", "17.03.2~ce-0~ubuntu-xenial"]
`
)
