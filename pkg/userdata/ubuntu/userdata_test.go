package ubuntu

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
			ccProvider:       &fakeCloudConfigProvider{name: "aws", config: "{aws-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DistUpgradeOnBoot: true},
			userdata:         docker12DistupgradeAWS,
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
			ccProvider:       &fakeCloudConfigProvider{name: "", config: "", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DistUpgradeOnBoot: false},
			userdata:         CRIO19Digitalocean,
		},
		{
			name: "docker 17.03 openstack multiple dns",
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
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10"), net.ParseIP("10.10.10.11"), net.ParseIP("10.10.10.12")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DistUpgradeOnBoot: true},
			userdata:         docker1703DistupgradeOpenstackMultipleDNS,
		},
		{
			name: "docker 17.03 openstack kubelet v version prefix",
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
					Kubelet: "v1.9.2",
				},
			},
			ccProvider:       &fakeCloudConfigProvider{name: "openstack", config: "{openstack-config:true}", err: nil},
			DNSIPs:           []net.IP{net.ParseIP("10.10.10.10")},
			kubernetesCACert: "CACert",
			resErr:           nil,
			osConfig:         &Config{DistUpgradeOnBoot: true},
			userdata:         docker1703DistupgradeOpenstack,
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

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    for component in kubelet kubeadm; do
      if ! [[ -x /opt/bin/$component ]]; then
        curl -L --fail -o /opt/bin/$component https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/$component
        chmod +x /opt/bin/$component
      fi
    done
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://storage.googleapis.com/cni-plugins/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

    if ! [[ -f /etc/systemd/system/kubelet.service.d/10-kubeadm.conf ]]; then
      curl -L --fail https://raw.githubusercontent.com/kubernetes/kubernetes/v1.9.2/build/debs/10-kubeadm.conf \
        |sed "s:/usr/bin:/opt/bin:g" > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
      systemctl daemon-reload
    fi

- path: "/etc/systemd/system/kubernetes-binaries.service"
  content: |
    [Unit]
    Requires=network-online.target
    After=network-online.target
    Requires=docker.service
    After=docker.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/etc/kubernetes/download.sh

- path: "/etc/systemd/system/kubeadm-join.service"
  content: |
    [Unit]
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/sbin/modprobe br_netfilter
    ExecStart=/opt/bin/kubeadm join \
      --token my-token \
      --discovery-token-ca-cert-hash sha256:6caecce9fedcb55d4953d61a27dc6997361a2f226ad86d7e6004dde7526fc4b1 \
      --ignore-preflight-errors=Port-10250 \
      server:443

- path: "/etc/systemd/system/kubelet.service.d/20-extra.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS=--cloud-provider=aws --cloud-config=/etc/kubernetes/cloud-conf \
      "

- path: "/etc/systemd/system/kubelet.service.d/30-clusterdns.conf"
  content: |
    [Service]
    Environment="KUBELET_DNS_ARGS=--cluster-dns=10.10.10.10 --cluster-domain=cluster.local"

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service
    Requires=docker.service
    After=docker.service

    [Service]
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=/opt/bin/kubelet
    Restart=always
    StartLimitInterval=0
    RestartSec=10
    Restart=always

    [Install]
    WantedBy=multi-user.target

runcmd:
# Required for Hetzner, because they set some arbitrary password
# if the sshkey wasnt set via their API and require as to change
# that password on first login, which we cant do since we dont know
# it
- chage -d $(date +%s) root
- systemctl enable kubelet
- systemctl start kubeadm-join

apt:
  sources:
    docker:
      source: deb [arch=amd64] https://download.docker.com/linux/ubuntu $RELEASE stable
      key: |
        -----BEGIN PGP PUBLIC KEY BLOCK-----

        mQINBFit2ioBEADhWpZ8/wvZ6hUTiXOwQHXMAlaFHcPH9hAtr4F1y2+OYdbtMuth
        lqqwp028AqyY+PRfVMtSYMbjuQuu5byyKR01BbqYhuS3jtqQmljZ/bJvXqnmiVXh
        38UuLa+z077PxyxQhu5BbqntTPQMfiyqEiU+BKbq2WmANUKQf+1AmZY/IruOXbnq
        L4C1+gJ8vfmXQt99npCaxEjaNRVYfOS8QcixNzHUYnb6emjlANyEVlZzeqo7XKl7
        UrwV5inawTSzWNvtjEjj4nJL8NsLwscpLPQUhTQ+7BbQXAwAmeHCUTQIvvWXqw0N
        cmhh4HgeQscQHYgOJjjDVfoY5MucvglbIgCqfzAHW9jxmRL4qbMZj+b1XoePEtht
        ku4bIQN1X5P07fNWzlgaRL5Z4POXDDZTlIQ/El58j9kp4bnWRCJW0lya+f8ocodo
        vZZ+Doi+fy4D5ZGrL4XEcIQP/Lv5uFyf+kQtl/94VFYVJOleAv8W92KdgDkhTcTD
        G7c0tIkVEKNUq48b3aQ64NOZQW7fVjfoKwEZdOqPE72Pa45jrZzvUFxSpdiNk2tZ
        XYukHjlxxEgBdC/J3cMMNRE1F4NCA3ApfV1Y7/hTeOnmDuDYwr9/obA8t016Yljj
        q5rdkywPf4JF8mXUW5eCN1vAFHxeg9ZWemhBtQmGxXnw9M+z6hWwc6ahmwARAQAB
        tCtEb2NrZXIgUmVsZWFzZSAoQ0UgZGViKSA8ZG9ja2VyQGRvY2tlci5jb20+iQI3
        BBMBCgAhBQJYrefAAhsvBQsJCAcDBRUKCQgLBRYCAwEAAh4BAheAAAoJEI2BgDwO
        v82IsskP/iQZo68flDQmNvn8X5XTd6RRaUH33kXYXquT6NkHJciS7E2gTJmqvMqd
        tI4mNYHCSEYxI5qrcYV5YqX9P6+Ko+vozo4nseUQLPH/ATQ4qL0Zok+1jkag3Lgk
        jonyUf9bwtWxFp05HC3GMHPhhcUSexCxQLQvnFWXD2sWLKivHp2fT8QbRGeZ+d3m
        6fqcd5Fu7pxsqm0EUDK5NL+nPIgYhN+auTrhgzhK1CShfGccM/wfRlei9Utz6p9P
        XRKIlWnXtT4qNGZNTN0tR+NLG/6Bqd8OYBaFAUcue/w1VW6JQ2VGYZHnZu9S8LMc
        FYBa5Ig9PxwGQOgq6RDKDbV+PqTQT5EFMeR1mrjckk4DQJjbxeMZbiNMG5kGECA8
        g383P3elhn03WGbEEa4MNc3Z4+7c236QI3xWJfNPdUbXRaAwhy/6rTSFbzwKB0Jm
        ebwzQfwjQY6f55MiI/RqDCyuPj3r3jyVRkK86pQKBAJwFHyqj9KaKXMZjfVnowLh
        9svIGfNbGHpucATqREvUHuQbNnqkCx8VVhtYkhDb9fEP2xBu5VvHbR+3nfVhMut5
        G34Ct5RS7Jt6LIfFdtcn8CaSas/l1HbiGeRgc70X/9aYx/V/CEJv0lIe8gP6uDoW
        FPIZ7d6vH+Vro6xuWEGiuMaiznap2KhZmpkgfupyFmplh0s6knymuQINBFit2ioB
        EADneL9S9m4vhU3blaRjVUUyJ7b/qTjcSylvCH5XUE6R2k+ckEZjfAMZPLpO+/tF
        M2JIJMD4SifKuS3xck9KtZGCufGmcwiLQRzeHF7vJUKrLD5RTkNi23ydvWZgPjtx
        Q+DTT1Zcn7BrQFY6FgnRoUVIxwtdw1bMY/89rsFgS5wwuMESd3Q2RYgb7EOFOpnu
        w6da7WakWf4IhnF5nsNYGDVaIHzpiqCl+uTbf1epCjrOlIzkZ3Z3Yk5CM/TiFzPk
        z2lLz89cpD8U+NtCsfagWWfjd2U3jDapgH+7nQnCEWpROtzaKHG6lA3pXdix5zG8
        eRc6/0IbUSWvfjKxLLPfNeCS2pCL3IeEI5nothEEYdQH6szpLog79xB9dVnJyKJb
        VfxXnseoYqVrRz2VVbUI5Blwm6B40E3eGVfUQWiux54DspyVMMk41Mx7QJ3iynIa
        1N4ZAqVMAEruyXTRTxc9XW0tYhDMA/1GYvz0EmFpm8LzTHA6sFVtPm/ZlNCX6P1X
        zJwrv7DSQKD6GGlBQUX+OeEJ8tTkkf8QTJSPUdh8P8YxDFS5EOGAvhhpMBYD42kQ
        pqXjEC+XcycTvGI7impgv9PDY1RCC1zkBjKPa120rNhv/hkVk/YhuGoajoHyy4h7
        ZQopdcMtpN2dgmhEegny9JCSwxfQmQ0zK0g7m6SHiKMwjwARAQABiQQ+BBgBCAAJ
        BQJYrdoqAhsCAikJEI2BgDwOv82IwV0gBBkBCAAGBQJYrdoqAAoJEH6gqcPyc/zY
        1WAP/2wJ+R0gE6qsce3rjaIz58PJmc8goKrir5hnElWhPgbq7cYIsW5qiFyLhkdp
        YcMmhD9mRiPpQn6Ya2w3e3B8zfIVKipbMBnke/ytZ9M7qHmDCcjoiSmwEXN3wKYI
        mD9VHONsl/CG1rU9Isw1jtB5g1YxuBA7M/m36XN6x2u+NtNMDB9P56yc4gfsZVES
        KA9v+yY2/l45L8d/WUkUi0YXomn6hyBGI7JrBLq0CX37GEYP6O9rrKipfz73XfO7
        JIGzOKZlljb/D9RX/g7nRbCn+3EtH7xnk+TK/50euEKw8SMUg147sJTcpQmv6UzZ
        cM4JgL0HbHVCojV4C/plELwMddALOFeYQzTif6sMRPf+3DSj8frbInjChC3yOLy0
        6br92KFom17EIj2CAcoeq7UPhi2oouYBwPxh5ytdehJkoo+sN7RIWua6P2WSmon5
        U888cSylXC0+ADFdgLX9K2zrDVYUG1vo8CX0vzxFBaHwN6Px26fhIT1/hYUHQR1z
        VfNDcyQmXqkOnZvvoMfz/Q0s9BhFJ/zU6AgQbIZE/hm1spsfgvtsD1frZfygXJ9f
        irP+MSAI80xHSf91qSRZOj4Pl3ZJNbq4yYxv0b1pkMqeGdjdCYhLU+LZ4wbQmpCk
        SVe2prlLureigXtmZfkqevRz7FrIZiu9ky8wnCAPwC7/zmS18rgP/17bOtL4/iIz
        QhxAAoAMWVrGyJivSkjhSGx1uCojsWfsTAm11P7jsruIL61ZzMUVE2aM3Pmj5G+W
        9AcZ58Em+1WsVnAXdUR//bMmhyr8wL/G1YO1V3JEJTRdxsSxdYa4deGBBY/Adpsw
        24jxhOJR+lsJpqIUeb999+R8euDhRHG9eFO7DRu6weatUJ6suupoDTRWtr/4yGqe
        dKxV3qQhNLSnaAzqW/1nA3iUB4k7kCaKZxhdhDbClf9P37qaRW467BLCVO/coL3y
        Vm50dwdrNtKpMBh3ZpbB1uJvgi9mXtyBOMJ3v8RZeDzFiG8HdCtg9RvIt/AIFoHR
        H3S+U79NT6i0KPzLImDfs8T7RlpyuMc4Ufs8ggyg9v3Ae6cN3eQyxcK3w0cbBwsh
        /nQNfsA6uu+9H7NhbehBMhYnpNZyrHzCmzyXkauwRAqoCbGCNykTRwsur9gS41TQ
        M8ssD1jFheOJf3hODnkKU+HKjvMROl1DK7zdmLdNzA1cvtZH/nCC9KPj1z8QC47S
        xx+dTZSx4ONAhwbS/LN3PoKtn8LPjY9NP9uDWI+TWYquS2U+KHDrBDlsgozDbs/O
        jCxcpDzNmXpWQHEtHU7649OXHP7UeNST1mCUCH5qdank0V1iejF6/CfTFU4MfcrG
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
- "open-vm-tools"
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


- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    for component in kubelet kubeadm; do
      if ! [[ -x /opt/bin/$component ]]; then
        curl -L --fail -o /opt/bin/$component https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/$component
        chmod +x /opt/bin/$component
      fi
    done
    if ! [[ -x /opt/bin/crictl ]]; then
      curl -L --fail https://github.com/kubernetes-incubator/cri-tools/releases/download/v0.2/crictl-v0.2-linux-amd64.tar.gz |tar -xzC /opt/bin
    fi
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://storage.googleapis.com/cni-plugins/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

    if ! [[ -f /etc/systemd/system/kubelet.service.d/10-kubeadm.conf ]]; then
      curl -L --fail https://raw.githubusercontent.com/kubernetes/kubernetes/v1.9.2/build/debs/10-kubeadm.conf \
        |sed "s:/usr/bin:/opt/bin:g" > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
      systemctl daemon-reload
    fi

- path: "/etc/systemd/system/kubernetes-binaries.service"
  content: |
    [Unit]
    Requires=network-online.target
    After=network-online.target
    Requires=crio.service
    After=crio.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/etc/kubernetes/download.sh

- path: "/etc/systemd/system/kubeadm-join.service"
  content: |
    [Unit]
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/sbin/modprobe br_netfilter
    ExecStart=/opt/bin/kubeadm join \
      --cri-socket /var/run/crio/crio.sock \
      --token my-token \
      --discovery-token-ca-cert-hash sha256:6caecce9fedcb55d4953d61a27dc6997361a2f226ad86d7e6004dde7526fc4b1 \
      --ignore-preflight-errors=Port-10250 \
      server:443

- path: "/etc/systemd/system/kubelet.service.d/20-extra.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS= \
       --container-runtime=remote --container-runtime-endpoint=unix:///var/run/crio/crio.sock --cgroup-driver=systemd"

- path: "/etc/systemd/system/kubelet.service.d/30-clusterdns.conf"
  content: |
    [Service]
    Environment="KUBELET_DNS_ARGS=--cluster-dns=10.10.10.10 --cluster-domain=cluster.local"

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service
    Requires=crio.service
    After=crio.service

    [Service]
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=/opt/bin/kubelet
    Restart=always
    StartLimitInterval=0
    RestartSec=10
    Restart=always

    [Install]
    WantedBy=multi-user.target
- path: "/etc/sysconfig/crio-network"
  content: |
    CRIO_NETWORK_OPTIONS="--registry=docker.io"

runcmd:
# Required for Hetzner, because they set some arbitrary password
# if the sshkey wasnt set via their API and require as to change
# that password on first login, which we cant do since we dont know
# it
- chage -d $(date +%s) root
- systemctl enable kubelet
- systemctl start kubeadm-join

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
- "open-vm-tools"
- "cri-o-1.9"
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

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    for component in kubelet kubeadm; do
      if ! [[ -x /opt/bin/$component ]]; then
        curl -L --fail -o /opt/bin/$component https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/$component
        chmod +x /opt/bin/$component
      fi
    done
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://storage.googleapis.com/cni-plugins/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

    if ! [[ -f /etc/systemd/system/kubelet.service.d/10-kubeadm.conf ]]; then
      curl -L --fail https://raw.githubusercontent.com/kubernetes/kubernetes/v1.9.2/build/debs/10-kubeadm.conf \
        |sed "s:/usr/bin:/opt/bin:g" > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
      systemctl daemon-reload
    fi

- path: "/etc/systemd/system/kubernetes-binaries.service"
  content: |
    [Unit]
    Requires=network-online.target
    After=network-online.target
    Requires=docker.service
    After=docker.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/etc/kubernetes/download.sh

- path: "/etc/systemd/system/kubeadm-join.service"
  content: |
    [Unit]
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/sbin/modprobe br_netfilter
    ExecStart=/opt/bin/kubeadm join \
      --token my-token \
      --discovery-token-ca-cert-hash sha256:6caecce9fedcb55d4953d61a27dc6997361a2f226ad86d7e6004dde7526fc4b1 \
      --ignore-preflight-errors=Port-10250 \
      server:443

- path: "/etc/systemd/system/kubelet.service.d/20-extra.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS=--cloud-provider=openstack --cloud-config=/etc/kubernetes/cloud-conf \
      "

- path: "/etc/systemd/system/kubelet.service.d/30-clusterdns.conf"
  content: |
    [Service]
    Environment="KUBELET_DNS_ARGS=--cluster-dns=10.10.10.10 --cluster-domain=cluster.local"

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service
    Requires=docker.service
    After=docker.service

    [Service]
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=/opt/bin/kubelet
    Restart=always
    StartLimitInterval=0
    RestartSec=10
    Restart=always

    [Install]
    WantedBy=multi-user.target

runcmd:
# Required for Hetzner, because they set some arbitrary password
# if the sshkey wasnt set via their API and require as to change
# that password on first login, which we cant do since we dont know
# it
- chage -d $(date +%s) root
- systemctl enable kubelet
- systemctl start kubeadm-join

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
- "open-vm-tools"
- ["docker-ce", "17.03.2~ce-0~ubuntu-xenial"]
`
	docker1703DistupgradeOpenstackMultipleDNS = `#cloud-config
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

- path: "/etc/kubernetes/download.sh"
  permissions: '0777'
  content: |
    #!/bin/bash
    set -xeuo pipefail
    mkdir -p /opt/bin /opt/cni/bin /etc/cni/net.d /var/run/kubernetes /var/lib/kubelet /etc/kubernetes/manifests /var/log/containers
    for component in kubelet kubeadm; do
      if ! [[ -x /opt/bin/$component ]]; then
        curl -L --fail -o /opt/bin/$component https://storage.googleapis.com/kubernetes-release/release/v1.9.2/bin/linux/amd64/$component
        chmod +x /opt/bin/$component
      fi
    done
    if [ ! -f /opt/cni/bin/bridge ]; then
      curl -L -o /opt/cni.tgz https://storage.googleapis.com/cni-plugins/cni-plugins-amd64-v0.6.0.tgz
      mkdir -p /opt/cni/bin/
      tar -xzf /opt/cni.tgz -C /opt/cni/bin/
    fi

    if ! [[ -f /etc/systemd/system/kubelet.service.d/10-kubeadm.conf ]]; then
      curl -L --fail https://raw.githubusercontent.com/kubernetes/kubernetes/v1.9.2/build/debs/10-kubeadm.conf \
        |sed "s:/usr/bin:/opt/bin:g" > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
      systemctl daemon-reload
    fi

- path: "/etc/systemd/system/kubernetes-binaries.service"
  content: |
    [Unit]
    Requires=network-online.target
    After=network-online.target
    Requires=docker.service
    After=docker.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/etc/kubernetes/download.sh

- path: "/etc/systemd/system/kubeadm-join.service"
  content: |
    [Unit]
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service

    [Service]
    Type=oneshot
    RemainAfterExit=true
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStartPre=/sbin/modprobe br_netfilter
    ExecStart=/opt/bin/kubeadm join \
      --token my-token \
      --discovery-token-ca-cert-hash sha256:6caecce9fedcb55d4953d61a27dc6997361a2f226ad86d7e6004dde7526fc4b1 \
      --ignore-preflight-errors=Port-10250 \
      server:443

- path: "/etc/systemd/system/kubelet.service.d/20-extra.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS=--cloud-provider=openstack --cloud-config=/etc/kubernetes/cloud-conf \
      "

- path: "/etc/systemd/system/kubelet.service.d/30-clusterdns.conf"
  content: |
    [Service]
    Environment="KUBELET_DNS_ARGS=--cluster-dns=10.10.10.10,10.10.10.11,10.10.10.12 --cluster-domain=cluster.local"

- path: "/etc/systemd/system/kubelet.service"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target kubernetes-binaries.service
    After=network-online.target kubernetes-binaries.service
    Requires=docker.service
    After=docker.service

    [Service]
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=/opt/bin/kubelet
    Restart=always
    StartLimitInterval=0
    RestartSec=10
    Restart=always

    [Install]
    WantedBy=multi-user.target

runcmd:
# Required for Hetzner, because they set some arbitrary password
# if the sshkey wasnt set via their API and require as to change
# that password on first login, which we cant do since we dont know
# it
- chage -d $(date +%s) root
- systemctl enable kubelet
- systemctl start kubeadm-join

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
- "open-vm-tools"
- ["docker-ce", "17.03.2~ce-0~ubuntu-xenial"]
`
)
