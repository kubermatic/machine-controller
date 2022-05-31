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

package helper

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	kubeletv1b1 "k8s.io/kubelet/config/v1beta1"
	"k8s.io/utils/pointer"
	kyaml "sigs.k8s.io/yaml"
)

const (
	defaultKubeletContainerLogMaxSize = "100Mi"
)

const (
	kubeletFlagsTpl = `--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf \
--kubeconfig=/var/lib/kubelet/kubeconfig \
--config=/etc/kubernetes/kubelet.conf \
--cert-dir=/etc/kubernetes/pki \
{{- if or (.CloudProvider) (.IsExternal) }}
{{ cloudProviderFlags .CloudProvider .IsExternal }} \
{{- end }}
{{- if and (.Hostname) (ne .CloudProvider "aws") }}
--hostname-override={{ .Hostname }} \
{{- else if and (eq .CloudProvider "aws") (.IsExternal) }}
--hostname-override=${KUBELET_HOSTNAME} \
{{- end }}
--exit-on-lock-contention \
--lock-file=/tmp/kubelet.lock \
{{- if .PauseImage }}
--pod-infra-container-image={{ .PauseImage }} \
{{- end }}
{{- if .InitialTaints }}
--register-with-taints={{- .InitialTaints }} \
{{- end }}
{{- range .ExtraKubeletFlags }}
{{ . }} \
{{- end }}
--node-ip ${KUBELET_NODE_IP}`

	kubeletSystemdUnitTpl = `[Unit]
After={{ .ContainerRuntime }}.service
Requires={{ .ContainerRuntime }}.service

Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/

[Service]
User=root
Restart=always
StartLimitInterval=0
RestartSec=10
CPUAccounting=true
MemoryAccounting=true

Environment="PATH=/opt/bin:/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin/"
EnvironmentFile=-/etc/environment

ExecStartPre=/bin/bash /opt/load-kernel-modules.sh
{{ if .DisableSwap }}
ExecStartPre=/bin/bash /opt/disable-swap.sh
{{ end }}
ExecStartPre=/bin/bash /opt/bin/setup_net_env.sh
ExecStart=/opt/bin/kubelet $KUBELET_EXTRA_ARGS \
{{ kubeletFlags .KubeletVersion .CloudProvider .Hostname .ClusterDNSIPs .IsExternal .PauseImage .InitialTaints .ExtraKubeletFlags | indent 2 }}

[Install]
WantedBy=multi-user.target`

	containerRuntimeHealthCheckSystemdUnitTpl = `[Unit]
Requires={{ .ContainerRuntime }}.service
After={{ .ContainerRuntime }}.service

[Service]
ExecStart=/opt/bin/health-monitor.sh container-runtime

[Install]
WantedBy=multi-user.target`
)

const cpFlags = `--cloud-provider=%s \
--cloud-config=/etc/kubernetes/cloud-config`

// List of allowed TLS cipher suites for kubelet.
var kubeletTLSCipherSuites = []string{
	// TLS 1.3 cipher suites
	"TLS_AES_128_GCM_SHA256",
	"TLS_AES_256_GCM_SHA384",
	"TLS_CHACHA20_POLY1305_SHA256",
	// TLS 1.0 - 1.2 cipher suites
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
}

// CloudProviderFlags returns --cloud-provider and --cloud-config flags.
func CloudProviderFlags(cpName string, external bool) (string, error) {
	if cpName == "" && !external {
		return "", nil
	}

	if external {
		return "--cloud-provider=external", nil
	}
	return fmt.Sprintf(cpFlags, cpName), nil
}

// KubeletSystemdUnit returns the systemd unit for the kubelet.
func KubeletSystemdUnit(containerRuntime, kubeletVersion, cloudProvider, hostname string, dnsIPs []net.IP, external bool, pauseImage string, initialTaints []corev1.Taint, extraKubeletFlags []string, disableSwap bool) (string, error) {
	tmpl, err := template.New("kubelet-systemd-unit").Funcs(TxtFuncMap()).Parse(kubeletSystemdUnitTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse kubelet-systemd-unit template: %w", err)
	}

	data := struct {
		ContainerRuntime  string
		KubeletVersion    string
		CloudProvider     string
		Hostname          string
		ClusterDNSIPs     []net.IP
		IsExternal        bool
		PauseImage        string
		InitialTaints     []corev1.Taint
		ExtraKubeletFlags []string
		DisableSwap       bool
	}{
		ContainerRuntime:  containerRuntime,
		KubeletVersion:    kubeletVersion,
		CloudProvider:     cloudProvider,
		Hostname:          hostname,
		ClusterDNSIPs:     dnsIPs,
		IsExternal:        external,
		PauseImage:        pauseImage,
		InitialTaints:     initialTaints,
		ExtraKubeletFlags: extraKubeletFlags,
		DisableSwap:       disableSwap,
	}

	var buf strings.Builder
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute kubelet-systemd-unit template: %w", err)
	}

	return buf.String(), nil
}

// kubeletConfiguration returns marshaled kubelet.config.k8s.io/v1beta1 KubeletConfiguration.
func kubeletConfiguration(clusterDomain string, clusterDNS []net.IP, featureGates map[string]bool, kubeletConfigs map[string]string, containerRuntime string) (string, error) {
	clusterDNSstr := make([]string, 0, len(clusterDNS))
	for _, ip := range clusterDNS {
		clusterDNSstr = append(clusterDNSstr, ip.String())
	}

	cfg := kubeletv1b1.KubeletConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeletConfiguration",
			APIVersion: kubeletv1b1.SchemeGroupVersion.String(),
		},
		Authentication: kubeletv1b1.KubeletAuthentication{
			X509: kubeletv1b1.KubeletX509Authentication{
				ClientCAFile: "/etc/kubernetes/pki/ca.crt",
			},
			Webhook: kubeletv1b1.KubeletWebhookAuthentication{
				Enabled: pointer.BoolPtr(true),
			},
			Anonymous: kubeletv1b1.KubeletAnonymousAuthentication{
				Enabled: pointer.BoolPtr(false),
			},
		},
		Authorization: kubeletv1b1.KubeletAuthorization{
			Mode: kubeletv1b1.KubeletAuthorizationModeWebhook,
		},
		CgroupDriver:          "systemd",
		ClusterDNS:            clusterDNSstr,
		ClusterDomain:         clusterDomain,
		FeatureGates:          featureGates,
		ProtectKernelDefaults: true,
		ReadOnlyPort:          0,
		RotateCertificates:    true,
		ServerTLSBootstrap:    true,
		StaticPodPath:         "/etc/kubernetes/manifests",
		KubeReserved:          map[string]string{"cpu": "200m", "memory": "200Mi", "ephemeral-storage": "1Gi"},
		SystemReserved:        map[string]string{"cpu": "200m", "memory": "200Mi", "ephemeral-storage": "1Gi"},
		EvictionHard:          map[string]string{"memory.available": "100Mi", "nodefs.available": "10%", "nodefs.inodesFree": "5%", "imagefs.available": "15%"},
		VolumePluginDir:       "/var/lib/kubelet/volumeplugins",
		TLSCipherSuites:       kubeletTLSCipherSuites,
		ContainerLogMaxSize:   defaultKubeletContainerLogMaxSize,
	}

	if kubeReserved, ok := kubeletConfigs[common.KubeReservedKubeletConfig]; ok {
		for _, krPair := range strings.Split(kubeReserved, ",") {
			krKV := strings.SplitN(krPair, "=", 2)
			if len(krKV) != 2 {
				continue
			}
			cfg.KubeReserved[krKV[0]] = krKV[1]
		}
	}

	if systemReserved, ok := kubeletConfigs[common.SystemReservedKubeletConfig]; ok {
		for _, srPair := range strings.Split(systemReserved, ",") {
			srKV := strings.SplitN(srPair, "=", 2)
			if len(srKV) != 2 {
				continue
			}
			cfg.SystemReserved[srKV[0]] = srKV[1]
		}
	}

	if evictionHard, ok := kubeletConfigs[common.EvictionHardKubeletConfig]; ok {
		for _, ehPair := range strings.Split(evictionHard, ",") {
			ehKV := strings.SplitN(ehPair, "<", 2)
			if len(ehKV) != 2 {
				continue
			}
			cfg.EvictionHard[ehKV[0]] = ehKV[1]
		}
	}

	if maxPods, ok := kubeletConfigs[common.MaxPodsKubeletConfig]; ok {
		mp, err := strconv.ParseInt(maxPods, 10, 32)
		if err != nil {
			// Instead of breaking the workflow, just print a warning and skip the configuration
			klog.Warningf("Skipping invalid MaxPods value %v for Kubelet configuration", maxPods)
		} else {
			cfg.MaxPods = int32(mp)
		}
	}

	if containerLogMaxSize, ok := kubeletConfigs[common.ContainerLogMaxSizeKubeletConfig]; ok {
		cfg.ContainerLogMaxSize = containerLogMaxSize
	}
	if containerLogMaxFiles, ok := kubeletConfigs[common.ContainerLogMaxFilesKubeletConfig]; ok {
		maxFiles, err := strconv.Atoi(containerLogMaxFiles)
		if err != nil || maxFiles < 0 {
			// Instead of breaking the workflow, just print a warning and skip the configuration
			klog.Warningf("Skipping invalid ContainerLogMaxSize value %v for Kubelet configuration", containerLogMaxFiles)
		} else {
			cfg.ContainerLogMaxFiles = pointer.Int32Ptr(int32(maxFiles))
		}
	}

	if enabled, ok := featureGates["SeccompDefault"]; ok && enabled {
		cfg.SeccompDefault = pointer.Bool(true)
	}

	buf, err := kyaml.Marshal(cfg)
	return string(buf), err
}

// KubeletFlags returns the kubelet flags.
func KubeletFlags(version, cloudProvider, hostname string, dnsIPs []net.IP, external bool, pauseImage string, initialTaints []corev1.Taint, extraKubeletFlags []string) (string, error) {
	tmpl, err := template.New("kubelet-flags").Funcs(TxtFuncMap()).Parse(kubeletFlagsTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse kubelet-flags template: %w", err)
	}

	initialTaintsArgs := []string{}
	for _, taint := range initialTaints {
		initialTaintsArgs = append(initialTaintsArgs, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
	}

	kubeletFlags := make([]string, len(extraKubeletFlags))
	copy(kubeletFlags, extraKubeletFlags)

	ver, err := semver.NewVersion(version)
	if err != nil {
		return "", err
	}
	con, err := semver.NewConstraint("< 1.23")
	if err != nil {
		return "", err
	}

	if con.Check(ver) {
		kubeletFlags = append(kubeletFlags,
			"--dynamic-config-dir=/etc/kubernetes/dynamic-config-dir",
			"--feature-gates=DynamicKubeletConfig=true",
		)
	}

	// --network-plugin was removed in 1.24 and can only be set for 1.23 or lower

	con, err = semver.NewConstraint("< 1.24")
	if err != nil {
		return "", err
	}

	if con.Check(ver) {
		kubeletFlags = append(kubeletFlags,
			"--network-plugin=cni",
		)
	}

	data := struct {
		CloudProvider     string
		Hostname          string
		ClusterDNSIPs     []net.IP
		KubeletVersion    string
		IsExternal        bool
		PauseImage        string
		InitialTaints     string
		ExtraKubeletFlags []string
	}{
		CloudProvider:     cloudProvider,
		Hostname:          hostname,
		ClusterDNSIPs:     dnsIPs,
		KubeletVersion:    version,
		IsExternal:        external,
		PauseImage:        pauseImage,
		InitialTaints:     strings.Join(initialTaintsArgs, ","),
		ExtraKubeletFlags: kubeletFlags,
	}

	var buf strings.Builder
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute kubelet-flags template: %w", err)
	}

	return buf.String(), nil
}

// KubeletHealthCheckSystemdUnit kubelet health checking systemd unit.
func KubeletHealthCheckSystemdUnit() string {
	return `[Unit]
Requires=kubelet.service
After=kubelet.service

[Service]
ExecStart=/opt/bin/health-monitor.sh kubelet

[Install]
WantedBy=multi-user.target
`
}

// ContainerRuntimeHealthCheckSystemdUnit container-runtime health checking systemd unit.
func ContainerRuntimeHealthCheckSystemdUnit(containerRuntime string) (string, error) {
	tmpl, err := template.New("container-runtime-healthcheck-systemd-unit").Funcs(TxtFuncMap()).Parse(containerRuntimeHealthCheckSystemdUnitTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse container-runtime-healthcheck-systemd-unit template: %w", err)
	}

	data := struct {
		ContainerRuntime string
	}{
		ContainerRuntime: containerRuntime,
	}

	var buf strings.Builder
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute container-runtime-healthcheck-systemd-unit template: %w", err)
	}
	return buf.String(), nil
}
