package migrate

import (
	"encoding/json"
	"fmt"

	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	machinev1alpha1upstream "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	ContainerRuntimeInfoAnnotation = "machine-controller.kubermatic.io/container-runtime-info"
)

type machineV1alpha1Common struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   machineSpecV1alpha1Common             `json:"spec,omitempty"`
	Status machinev1alpha1upstream.MachineStatus `json:"status,omitempty"`
}

type machineSpecV1alpha1Common struct {
	// This ObjectMeta will autopopulate the Node created. Use this to
	// indicate what labels, annotations, name prefix, etc., should be used
	// when creating the Node.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The full, authoritative list of taints to apply to the corresponding
	// Node. This list will overwrite any modifications made to the Node on
	// an ongoing basis.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Provider-specific configuration to use during node creation.
	// +optional
	ProviderConfig map[string]interface{} `json:"providerConfig"`

	// A list of roles for this Machine to use.
	Roles []clustercommon.MachineRole `json:"roles,omitempty"`

	// Versions of key software to use. This field is optional at cluster
	// creation time, and omitting the field indicates that the cluster
	// installation tool should select defaults for the user. These
	// defaults may differ based on the cluster installer, but the tool
	// should populate the values it uses when persisting Machine objects.
	// A Machine spec missing this field at runtime is invalid.
	// +optional
	Versions machinev1alpha1upstream.MachineVersionInfo `json:"versions,omitempty"`

	// To populate in the associated Node for dynamic kubelet config. This
	// field already exists in Node, so any updates to it in the Machine
	// spec will be automatially copied to the linked NodeRef from the
	// status. The rest of dynamic kubelet config support should then work
	// as-is.
	// +optional
	ConfigSource *corev1.NodeConfigSource `json:"configSource,omitempty"`
}

//func Migrate(machine machineV1alpha1Common) (*machinev1alpha1upstream.Machine, error) {
//	isDownstreamMachine := checkIfIsDownstreamMachine(machine)
//	if isDownstreamMachine {
//		return migrateMachine(machine)
//	}
//	return castMachine(machine)
//}
//
//func castMachine(in machineV1alpha1Common) (*machinev1alpha1upstream.Machine, error) {
//	return nil, nil
//}

func migrateMachine(in machinev1alpha1downstream.Machine) (*machinev1alpha1upstream.Machine, error) {
	out := &machinev1alpha1upstream.Machine{}
	out.ObjectMeta = in.ObjectMeta

	// sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1.MachineStatus and
	// pkg/machines/v1alpha1.MachineStatus are semantically identical, the former
	// only has one additional field, so we cast by serializing and deserializing
	inStatusJSON, err := json.Marshal(in.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal downstreammachine status: %v", err)
	}
	if err = json.Unmarshal(inStatusJSON, &out.Status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal downstreammachine status: %v", err)
	}

	out.Spec.ObjectMeta = in.Spec.ObjectMeta
	out.Spec.Taints = in.Spec.Taints

	providerConfigRaw, err := json.Marshal(in.Spec.ProviderConfig)
	if err != nil {
		return nil, err
	}
	out.Spec.ProviderConfig = machinev1alpha1upstream.ProviderConfig{Value: &runtime.RawExtension{Raw: providerConfigRaw}}

	for _, inRole := range in.Spec.Roles {
		if inRole == machinev1alpha1downstream.MasterRole {
			out.Spec.Roles = append(out.Spec.Roles, clustercommon.MasterRole)
		}
		if inRole == machinev1alpha1downstream.NodeRole {
			out.Spec.Roles = append(out.Spec.Roles, clustercommon.NodeRole)
		}
	}

	// This currently results in in.Spec.Versions.ContainerRuntime being dropped,
	// because it does not exist in the upstream type
	// We work around this by writing it to an annotation
	inMachineVersionJSON, err := json.Marshal(in.Spec.Versions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal downstreammachine version: %v", err)
	}
	inContainerRuntimeInfoJSON, err := json.Marshal(in.Spec.Versions.ContainerRuntime)
	if err != nil {
		return nil, err
	}
	if out.ObjectMeta.Annotations == nil {
		out.ObjectMeta.Annotations = map[string]string{}
	}
	out.ObjectMeta.Annotations[ContainerRuntimeInfoAnnotation] = string(inContainerRuntimeInfoJSON)
	if err = json.Unmarshal(inMachineVersionJSON, &out.Spec.Versions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal downstreammachine version: %v", err)
	}

	out.Spec.ConfigSource = in.Spec.ConfigSource

	return out, err
}

func checkIfIsDownstreamMachine(machine machineV1alpha1Common) bool {
	_, valueFieldExists := machine.Spec.ProviderConfig["value"]
	_, valueFromFieldExsists := machine.Spec.ProviderConfig["valueFrom"]
	return valueFieldExists || valueFromFieldExsists
}
