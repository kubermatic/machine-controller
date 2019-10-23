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

package conversions

import (
	"encoding/json"
	"fmt"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const (
	TypeRevisionAnnotationName = "machine-controller/machine-type-revision"

	TypeRevisionCurrentVersion = "2ec456177c0e8f0a903f4e746d44baaae54cc591"
)

func Convert_MachinesV1alpha1Machine_To_ClusterV1alpha1Machine(in *machinesv1alpha1.Machine, out *clusterv1alpha1.Machine) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ObjectMeta = in.Spec.ObjectMeta
	out.SelfLink = ""
	out.UID = ""
	out.ResourceVersion = ""
	out.Generation = 0
	out.CreationTimestamp = metav1.Time{}
	out.ObjectMeta.Namespace = metav1.NamespaceSystem

	// Add annotation that indicates the current revision used for the types
	if out.Annotations == nil {
		out.Annotations = map[string]string{}
	}
	out.Annotations[TypeRevisionAnnotationName] = TypeRevisionCurrentVersion

	// github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1.MachineStatus and
	// pkg/machines/v1alpha1.MachineStatus are semantically identical, the former
	// only has one additional field, so we cast by serializing and deserializing
	inStatusJSON, err := json.Marshal(in.Status)
	if err != nil {
		return fmt.Errorf("failed to marshal downstreammachine status: %v", err)
	}
	if err = json.Unmarshal(inStatusJSON, &out.Status); err != nil {
		return fmt.Errorf("failed to unmarshal downstreammachine status: %v", err)
	}
	out.Spec.ObjectMeta = in.Spec.ObjectMeta
	out.Spec.Taints = in.Spec.Taints
	providerConfigRaw, err := json.Marshal(in.Spec.ProviderConfig)
	if err != nil {
		return err
	}
	out.Spec.ProviderSpec = clusterv1alpha1.ProviderSpec{Value: &runtime.RawExtension{Raw: providerConfigRaw}}

	// This currently results in in.Spec.Versions.ContainerRuntime being dropped,
	// because it was removed from the upstream type in
	// https://github.com/kubernetes-sigs/cluster-api/pull/240
	// To work around this, we put it into the providerConfig
	inMachineVersionJSON, err := json.Marshal(in.Spec.Versions)
	if err != nil {
		return fmt.Errorf("failed to marshal downstreammachine version: %v", err)
	}
	if err = json.Unmarshal(inMachineVersionJSON, &out.Spec.Versions); err != nil {
		return fmt.Errorf("failed to unmarshal downstreammachine version: %v", err)
	}
	out.Spec.ConfigSource = in.Spec.ConfigSource
	return nil
}
