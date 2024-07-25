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

const (
	Staged         string = "Staged"
	Provisioned           = "Provisioned"
	Decommissioned        = "Decommissioned"
)

const ()

// TinkerbellPluginSpec defines the required information for the Tinkerbell plugin.
type TinkerbellPluginSpec struct {

	// ClusterName specifies the name of the Tinkerbell cluster. This is used to identify
	// the cluster within a larger infrastructure or across multiple clusters.
	ClusterName providerconfigtypes.ConfigVarString `json:"clusterName"`

	// Auth contains the kubeconfig credentials needed to authenticate against the
	// Tinkerbell cluster API. This field is optional and should be provided if authentication is required.
	Auth Auth `json:"auth,omitempty"`

	// HegelURL specifies the URL of the Hegel metadata server. This server is crucial
	// for the cloud-init process as it provides necessary metadata to the booting machines.
	HegelURL providerconfigtypes.ConfigVarString `json:"hegelUrl"`

	// OSImageURL is the URL where the OS image for the Tinkerbell template is located.
	// This URL is used to download and stream the OS image during the provisioning process.
	OSImageURL providerconfigtypes.ConfigVarString `json:"osImageUrl"`

	// HardwareRef specifies the unique identifier of a single hardware object in the user-cluster
	// that corresponds to the machine deployment. This ensures a one-to-one mapping between a deployment
	// and a hardware object in the Tinkerbell cluster.
	HardwareRef types.NamespacedName `json:"hardwareRef"`
}

// Auth.
type Auth struct {
	Kubeconfig providerconfigtypes.ConfigVarString `json:"kubeconfig,omitempty"`
}

type Config struct {
	Kubeconfig  string
	ClusterName string
	RestConfig  *rest.Config
	HegelURL    string
	OSImageURL  string
}
