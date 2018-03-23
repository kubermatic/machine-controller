package centos

import (
	"net"
	"testing"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func TestUserDataGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec           machinesv1alpha1.MachineSpec
		kubeconfig     string
		ccProvider     cloud.ConfigProvider
		clusterDNSIPs  []net.IP
		expectedResult string
	}{
		{
			spec: machinesv1alpha1.MachineSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Versions: machinesv1alpha1.MachineVersionInfo{
					ContainerRuntime: machinesv1alpha1.ContainerRuntimeInfo{
						Name:    "docker",
						Version: "1.13",
					},
					Kubelet: "1.9.6",
				},
			},
			expectedResult: expectedResultDocker113,
		},
	}

	cloudProvider := &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil}
	kubeconfig := "kubeconfig"
	kubernetesCACert := "CACert"
	provider := Provider{}
	for _, test := range tests {
		userdata, err := provider.UserData(test.spec, kubeconfig, cloudProvider, test.clusterDNSIPs, kubernetesCACert)
		if err != nil {
			t.Errorf("error getting userdata: '%v'", err)
		}
		if test.expectedResult != userdata {
			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(test.expectedResult),
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

var (
	expectedResultDocker113 = `#cloud-config
hostname: node1



write_files:
- path: "/etc/yum.repos.d/kubernetes.repo"
  content: |
    [kubernetes]
    name=Kubernetes
    baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-$basearch
    enabled=1
    gpgcheck=1
    repo_gpgcheck=1
    gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg

- path: /etc/sysconfig/selinux
  content: |
    # This file controls the state of SELinux on the system.
    # SELINUX= can take one of these three values:
    #     enforcing - SELinux security policy is enforced.
    #     permissive - SELinux prints warnings instead of enforcing.
    #     disabled - No SELinux policy is loaded.
    SELINUX=permissive
    # SELINUXTYPE= can take one of three two values:
    #     targeted - Targeted processes are protected,
    #     minimum - Modification of targeted policy. Only selected processes are protected.
    #     mls - Multi Level Security protection.
    SELINUXTYPE=targeted
- path: "/etc/kubernetes/cloud-config"
  content: |
    {aws-config:true}

- path: "/etc/kubernetes/bootstrap.kubeconfig"
  content: |
    kubeconfig

- path: /etc/kubernetes/ca.crt
  content: |
    CACert

- path: "/etc/systemd/system/kubelet.service.d/10-machine-controller.conf"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target docker.service
    After=docker.service network-online.target

    [Service]
    Restart=always
    RestartSec=10
    StartLimitInterval=600
    StartLimitBurst=50
    TimeoutStartSec=5min
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=
    ExecStart=/bin/kubelet \
      --container-runtime=docker \
      --cgroup-driver=systemd \
      --allow-privileged=true \
      --cni-bin-dir=/opt/cni/bin \
      --cni-conf-dir=/etc/cni/net.d \
      --cluster-dns= \
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
      --exit-on-lock-contention \
      --read-only-port 0 \
      --authorization-mode=Webhook \
      --anonymous-auth=false \
      --client-ca-file=/etc/kubernetes/ca.crt

    [Install]
    WantedBy=multi-user.target

runcmd:
- setenforce 0 || true
- chage -d $(date +%s) root
- systemctl enable --now kubelet

packages:
- docker-1.13.1
- kubelet-1.9.6
- ebtables
- ethtool
- nfs-utils
- bash-completion # Have mercy for the poor operators
- sudo
`
)
