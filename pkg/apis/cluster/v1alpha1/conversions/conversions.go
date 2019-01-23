package conversions

import (
	"encoding/json"
	"fmt"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const (
	TypeRevisionAnnotationName = "machine-controller/machine-type-revision"

	TypeRevisionCurrentVersion = "45f1c93260140936c610e56575d7505ba3d52444"
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

	// sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1.MachineStatus and
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
