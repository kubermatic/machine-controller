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
	"net"
	"regexp"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"

	corev1 "k8s.io/api/core/v1"
)

// TxtFuncMap returns an aggregated template function map. Currently (custom functions + sprig).
func TxtFuncMap(log *zap.SugaredLogger) template.FuncMap {
	funcMap := sprig.TxtFuncMap()

	// use inline wrappers to inject the logger without forcing the templates to keep track of it

	funcMap["downloadBinariesScript"] = func(kubeletVersion string, downloadKubelet bool) (string, error) {
		return DownloadBinariesScript(log, kubeletVersion, downloadKubelet)
	}

	funcMap["safeDownloadBinariesScript"] = func(kubeVersion string) (string, error) {
		return SafeDownloadBinariesScript(log, kubeVersion)
	}

	funcMap["kubeletSystemdUnit"] = func(containerRuntime, kubeletVersion, cloudProvider, hostname string, dnsIPs []net.IP, external bool, ipFamily util.IPFamily, pauseImage string, initialTaints []corev1.Taint, extraKubeletFlags []string, disableSwap bool) (string, error) {
		return KubeletSystemdUnit(log, containerRuntime, kubeletVersion, cloudProvider, hostname, dnsIPs, external, ipFamily, pauseImage, initialTaints, extraKubeletFlags, disableSwap)
	}

	funcMap["kubeletConfiguration"] = func(clusterDomain string, clusterDNS []net.IP, featureGates map[string]bool, kubeletConfigs map[string]string, containerRuntime string) (string, error) {
		return kubeletConfiguration(log, clusterDomain, clusterDNS, featureGates, kubeletConfigs, containerRuntime)
	}

	funcMap["kubeletFlags"] = func(version, cloudProvider, hostname string, dnsIPs []net.IP, external bool, ipFamily util.IPFamily, pauseImage string, initialTaints []corev1.Taint, extraKubeletFlags []string) (string, error) {
		return KubeletFlags(log, version, cloudProvider, hostname, dnsIPs, external, ipFamily, pauseImage, initialTaints, extraKubeletFlags)
	}

	funcMap["containerRuntimeHealthCheckSystemdUnit"] = func(containerRuntime string) (string, error) {
		return ContainerRuntimeHealthCheckSystemdUnit(log, containerRuntime)
	}

	funcMap["cloudProviderFlags"] = CloudProviderFlags
	funcMap["kernelModulesScript"] = LoadKernelModulesScript
	funcMap["kernelSettings"] = KernelSettings
	funcMap["journalDConfig"] = JournalDConfig
	funcMap["kubeletHealthCheckSystemdUnit"] = KubeletHealthCheckSystemdUnit
	funcMap["dockerConfig"] = DockerConfig
	funcMap["proxyEnvironment"] = ProxyEnvironment
	funcMap["sshConfigAddendum"] = SSHConfigAddendum

	return funcMap
}

// CleanupTemplateOutput postprocesses the output of the template processing. Those
// may exist due to the working of template functions like those of the sprig package
// or template condition.
func CleanupTemplateOutput(output string) (string, error) {
	// Valid YAML files are not allowed to have empty lines containing spaces or tabs.
	// So far only cleanup.
	woBlankLines := regexp.MustCompile(`(?m)^[ \t]+$`).ReplaceAllString(output, "")
	return woBlankLines, nil
}
