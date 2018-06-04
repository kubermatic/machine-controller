package migrate

import (
	"encoding/json"
	"fmt"

	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func ConvertV1alpha1DownStreamMachineToV1alpha1ClusterMachine(in machinev1alpha1downstream.Machine) (*clusterv1alpha1.Machine, error) {
	out := &clusterv1alpha1.Machine{}
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ObjectMeta = in.Spec.ObjectMeta
	out.SelfLink = ""
	out.UID = ""
	out.ResourceVersion = ""
	out.Generation = 0
	out.CreationTimestamp = metav1.Time{}
	out.ObjectMeta.Namespace = "kube-system"

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

	out.Spec.ProviderConfig = clusterv1alpha1.ProviderConfig{Value: &runtime.RawExtension{Raw: providerConfigRaw}}

	for _, inRole := range in.Spec.Roles {
		if inRole == machinev1alpha1downstream.MasterRole {
			out.Spec.Roles = append(out.Spec.Roles, clustercommon.MasterRole)
		}
		if inRole == machinev1alpha1downstream.NodeRole {
			out.Spec.Roles = append(out.Spec.Roles, clustercommon.NodeRole)
		}
	}

	out.Finalizers = in.Finalizers

	// This currently results in in.Spec.Versions.ContainerRuntime being dropped,
	// because it was removed from the upstream type in
	// https://github.com/kubernetes-sigs/cluster-api/pull/240
	// To work around this, we put it into the providerConfig
	inMachineVersionJSON, err := json.Marshal(in.Spec.Versions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal downstreammachine version: %v", err)
	}
	if err = json.Unmarshal(inMachineVersionJSON, &out.Spec.Versions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal downstreammachine version: %v", err)
	}

	providerConfigMap, err := addContainerRuntimeInfoToProviderConfig(*out.Spec.ProviderConfig.Value,
		in.Spec.Versions.ContainerRuntime)
	if err != nil {
		return nil, fmt.Errorf("failed to add containerRuntimeInfo to providerConfig: %v", err)
	}
	out.Spec.ProviderConfig.Value.Raw, err = json.Marshal(providerConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshall providerconfig after adding containerRuntimeInfo: %v", err)
	}

	out.Spec.ConfigSource = in.Spec.ConfigSource

	return out, err
}

func addContainerRuntimeInfoToProviderConfig(providerConfigValue runtime.RawExtension, containerRuntimeInfo machinev1alpha1downstream.ContainerRuntimeInfo) (map[string]interface{}, error) {
	providerConfigMap := map[string]interface{}{}
	if err := json.Unmarshal(providerConfigValue.Raw, &providerConfigMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshall provider config into map: %v", err)
	}
	// The JSON unmarshall makes the map a null pointer if providerConfigValue.Raw is empty
	if providerConfigMap == nil {
		providerConfigMap = map[string]interface{}{}
	}
	if val, ok := providerConfigMap["operatingSystemSpec"]; ok {
		if valMap, ok := val.(map[string]interface{}); ok {
			valMap["containerRuntimeInfo"] = containerRuntimeInfo
			providerConfigMap["operatingSystemSpec"] = valMap
			return providerConfigMap, nil
		}
	}
	providerConfigMap["operatingSystemSpec"] = map[string]machinev1alpha1downstream.ContainerRuntimeInfo{"containerRuntimeInfo": containerRuntimeInfo}
	return providerConfigMap, nil
}
