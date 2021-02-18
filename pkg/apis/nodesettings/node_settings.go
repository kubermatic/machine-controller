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

package nodesettings

import (
	"encoding/json"
	"errors"
	"net"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
)

const (
	// AnnotationNodeSettings holds the node settings used for the machine.
	AnnotationNodeSettings = "machine-controller.kubermatic.io/node-settings"
)

type NodeSettings struct {
	// Translates to --cluster-dns on the kubelet.
	ClusterDNSIPs []net.IP `json:"clusterDNSIPs,omitempty"`
	// If set, this proxy will be configured on all nodes.
	HTTPProxy string `json:"httpProxy,omitempty"`
	// If set this will be set as NO_PROXY on the node.
	NoProxy string `json:"noProxy,omitempty"`
	// If set, those registries will be configured as insecure on the container runtime.
	InsecureRegistries []string `json:"insecureRegistries,omitempty"`
	// If set, these mirrors will be take for pulling all required images on the node.
	RegistryMirrors []string `json:"registryMirrors,omitempty"`
	// Translates to --pod-infra-container-image on the kubelet. If not set, the kubelet will default it.
	PauseImage string `json:"pauseImage,omitempty"`
	// The hyperkube image to use. Currently only Container Linux and Flatcar Linux uses it.
	HyperkubeImage string `json:"hyperkubeImage,omitempty"`
	// The kubelet repository to use. Currently only Flatcar Linux uses it.
	KubeletRepository string `json:"kubeletRepository,omitempty"`
	// Translates to feature gates on the kubelet.
	// Default: RotateKubeletServerCertificate=true
	KubeletFeatureGates map[string]bool `json:"kubeletFeatureGates,omitempty"`
	// container runtime to install
	ContainerRuntime containerruntime.Config `json:"containerRuntime"`
	// whether an external cloud provider is used.
	ExternalCloudProvider bool `json:"externalCloudProvider"`
}

func (n NodeSettings) Set(machine *clusterv1alpha1.Machine) {
	out, _ := json.Marshal(n)
	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}
	machine.Annotations[AnnotationNodeSettings] = string(out)
}

func FromMachine(machine *clusterv1alpha1.Machine) (*NodeSettings, error) {
	ns := NodeSettings{}
	value, ok := machine.Annotations[AnnotationNodeSettings]
	if !ok {
		return nil, errors.New("node settings annotation not found")
	}
	return &ns, json.Unmarshal([]byte(value), &ns)
}
