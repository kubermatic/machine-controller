/*
Copyright 2024 The Machine Controller Authors.

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

package types

import (
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

// TinkerbellPluginSpec defines the required information for the Tinkerbell plugin.
type TinkerbellPluginSpec struct {

	// The ClusterName of Tinkerbell cluster.
	ClusterName providerconfigtypes.ConfigVarString `json:"clusterName"`

	// Auth will contains the kubeconfig
	Auth Auth `json:"auth,omitempty"`

	// HardwareRefs contains a list of hardware object names from the machine-controller cluster
	// that need to be managed in the Tinkerbell cluster.
	HardwareRefs []types.NamespacedName `json:"hardwareRefs"`
}

// Auth.
type Auth struct {
	Kubeconfig providerconfigtypes.ConfigVarString `json:"kubeconfig,omitempty"`
}

type Config struct {
	Kubeconfig  string
	ClusterName string
	RestConfig  *rest.Config
}
