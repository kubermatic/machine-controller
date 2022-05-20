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

package kubevirt

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	kubevirttypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/kubevirt/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	netutil "github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	controllerutil "github.com/kubermatic/machine-controller/pkg/controller/util"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	if err := kubevirtv1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add kubevirtv1 to scheme: %v", err)
	}
}

const (
	// topologyKeyHostname defines the topology key for the node hostname.
	topologyKeyHostname = "kubernetes.io/hostname"
	// machineDeploymentLabelKey defines the label key used to contains as value the MachineDeployment name
	// which machine comes from.
	machineDeploymentLabelKey = "md"
)

var supportedOS = map[providerconfigtypes.OperatingSystem]*struct{}{
	providerconfigtypes.OperatingSystemCentOS:     nil,
	providerconfigtypes.OperatingSystemUbuntu:     nil,
	providerconfigtypes.OperatingSystemRHEL:       nil,
	providerconfigtypes.OperatingSystemFlatcar:    nil,
	providerconfigtypes.OperatingSystemRockyLinux: nil,
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a Kubevirt provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Kubeconfig            string
	RestConfig            *rest.Config
	DNSConfig             *corev1.PodDNSConfig
	DNSPolicy             corev1.DNSPolicy
	CPUs                  string
	Memory                string
	Namespace             string
	OsImage               OSImage
	StorageClassName      string
	PVCSize               resource.Quantity
	FlavorName            string
	SecondaryDisks        []SecondaryDisks
	PodAffinityPreset     AffinityType
	PodAntiAffinityPreset AffinityType
	NodeAffinityPreset    NodeAffinityPreset
}

type AffinityType string

const (
	// Facade for podAffinity, podAntiAffinity, nodeAffinity, nodeAntiAffinity
	// HardAffinityType: affinity will include requiredDuringSchedulingIgnoredDuringExecution.
	hardAffinityType = "hard"
	// SoftAffinityType: affinity will include preferredDuringSchedulingIgnoredDuringExecution.
	softAffinityType = "soft"
	// NoAffinityType: affinity section will not be preset.
	noAffinityType = ""
)

func (p *provider) affinityType(affinityType providerconfigtypes.ConfigVarString) (AffinityType, error) {
	podAffinityPresetString, err := p.configVarResolver.GetConfigVarStringValue(affinityType)
	if err != nil {
		return "", fmt.Errorf(`failed to parse "podAffinityPreset" field: %w`, err)
	}
	switch strings.ToLower(podAffinityPresetString) {
	case string(hardAffinityType):
		return hardAffinityType, nil
	case string(softAffinityType):
		return softAffinityType, nil
	case string(noAffinityType):
		return noAffinityType, nil
	}

	return "", fmt.Errorf("unknown affinityType: %s", affinityType)
}

// NodeAffinityPreset.
type NodeAffinityPreset struct {
	Type   AffinityType
	Key    string
	Values []string
}

type SecondaryDisks struct {
	Size             resource.Quantity
	StorageClassName string
}

type OSImage struct {
	URL            string
	DataVolumeName string
}

type kubeVirtServer struct {
	vmi kubevirtv1.VirtualMachineInstance
}

func (k *kubeVirtServer) Name() string {
	return k.vmi.Name
}

func (k *kubeVirtServer) ID() string {
	return string(k.vmi.UID)
}

func (k *kubeVirtServer) Addresses() map[string]corev1.NodeAddressType {
	addresses := map[string]corev1.NodeAddressType{}
	for _, kvInterface := range k.vmi.Status.Interfaces {
		if address := strings.Split(kvInterface.IP, "/")[0]; address != "" {
			addresses[address] = corev1.NodeInternalIP
		}
	}
	return addresses
}

func (k *kubeVirtServer) Status() instance.Status {
	if k.vmi.Status.Phase == kubevirtv1.Running {
		return instance.StatusRunning
	}
	return instance.StatusUnknown
}

var _ instance.Instance = &kubeVirtServer{}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := kubevirttypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	config := Config{}

	// Kubeconfig was specified directly in the Machine/MachineDeployment CR. In this case we need to ensure that the value is base64 encoded.
	if rawConfig.Auth.Kubeconfig.Value != "" {
		val, err := base64.StdEncoding.DecodeString(rawConfig.Auth.Kubeconfig.Value)
		if err != nil {
			// An error here means that this is not a valid base64 string
			// We can be more explicit here with the error for visibility. Webhook will return this error if we hit this scenario.
			return nil, nil, fmt.Errorf("failed to decode base64 encoded kubeconfig. Expected value is a base64 encoded Kubeconfig in JSON or YAML format: %w", err)
		}
		config.Kubeconfig = string(val)
	} else {
		// Environment variable or secret reference was used for providing the value of kubeconfig
		// We have to be lenient in this case and allow unencoded values as well.
		config.Kubeconfig, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Auth.Kubeconfig, "KUBEVIRT_KUBECONFIG")
		if err != nil {
			return nil, nil, fmt.Errorf(`failed to get value of "kubeconfig" field: %w`, err)
		}
		val, err := base64.StdEncoding.DecodeString(config.Kubeconfig)
		// We intentionally ignore errors here with an assumption that an unencoded YAML or JSON must have been passed on
		// in this case.
		if err == nil {
			config.Kubeconfig = string(val)
		}
	}

	config.RestConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(config.Kubeconfig))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode kubeconfig: %w", err)
	}

	config.CPUs, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Template.CPUs)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "cpus" field: %w`, err)
	}
	config.Memory, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Template.Memory)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "memory" field: %w`, err)
	}
	config.Namespace = getNamespace()
	osImage, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Template.PrimaryDisk.OsImage)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "sourceURL" field: %w`, err)
	}
	if _, err = url.ParseRequestURI(osImage); err == nil {
		config.OsImage.URL = osImage
	} else {
		config.OsImage.DataVolumeName = osImage
	}
	pvcSize, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Template.PrimaryDisk.Size)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "pvcSize" field: %w`, err)
	}
	if config.PVCSize, err = resource.ParseQuantity(pvcSize); err != nil {
		return nil, nil, fmt.Errorf(`failed to parse value of "pvcSize" field: %w`, err)
	}
	config.StorageClassName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Template.PrimaryDisk.StorageClassName)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "storageClassName" field: %w`, err)
	}
	config.FlavorName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.Flavor.Name)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "flavor.name" field: %w`, err)
	}

	dnsPolicyString, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.VirtualMachine.DNSPolicy)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "dnsPolicy" field: %w`, err)
	}
	if dnsPolicyString != "" {
		config.DNSPolicy, err = dnsPolicy(dnsPolicyString)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get dns policy: %w", err)
		}
	}
	if rawConfig.VirtualMachine.DNSConfig != nil {
		config.DNSConfig = rawConfig.VirtualMachine.DNSConfig
	}
	config.SecondaryDisks = make([]SecondaryDisks, 0, len(rawConfig.VirtualMachine.Template.SecondaryDisks))
	for _, sd := range rawConfig.VirtualMachine.Template.SecondaryDisks {
		sdSizeString, err := p.configVarResolver.GetConfigVarStringValue(sd.Size)
		if err != nil {
			return nil, nil, fmt.Errorf(`failed to parse "secondaryDisks.size" field: %w`, err)
		}
		pvc, err := resource.ParseQuantity(sdSizeString)
		if err != nil {
			return nil, nil, fmt.Errorf(`failed to parse value of "secondaryDisks.size" field: %w`, err)
		}

		scString, err := p.configVarResolver.GetConfigVarStringValue(sd.StorageClassName)
		if err != nil {
			return nil, nil, fmt.Errorf(`failed to parse value of "secondaryDisks.storageClass" field: %w`, err)
		}
		config.SecondaryDisks = append(config.SecondaryDisks, SecondaryDisks{
			Size:             pvc,
			StorageClassName: scString,
		})
	}

	// Affinity/AntiAffinity
	config.PodAffinityPreset, err = p.affinityType(rawConfig.Affinity.PodAffinityPreset)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "podAffinityPreset" field: %w`, err)
	}
	config.PodAntiAffinityPreset, err = p.affinityType(rawConfig.Affinity.PodAntiAffinityPreset)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "podAntiAffinityPreset" field: %w`, err)
	}
	config.NodeAffinityPreset, err = p.parseNodeAffinityPreset(rawConfig.Affinity.NodeAffinityPreset)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "nodeAffinityPreset" field: %w`, err)
	}

	return &config, pconfig, nil
}

func (p *provider) parseNodeAffinityPreset(nodeAffinityPreset kubevirttypes.NodeAffinityPreset) (NodeAffinityPreset, error) {
	nodeAffinity := NodeAffinityPreset{}
	var err error
	nodeAffinity.Type, err = p.affinityType(nodeAffinityPreset.Type)
	if err != nil {
		return nodeAffinity, fmt.Errorf(`failed to parse "nodeAffinity.type" field: %w`, err)
	}
	nodeAffinity.Key, err = p.configVarResolver.GetConfigVarStringValue(nodeAffinityPreset.Key)
	if err != nil {
		return nodeAffinity, fmt.Errorf(`failed to parse "nodeAffinity.key" field: %w`, err)
	}
	nodeAffinity.Values = make([]string, len(nodeAffinityPreset.Values))
	for _, v := range nodeAffinityPreset.Values {
		valueString, err := p.configVarResolver.GetConfigVarStringValue(v)
		if err != nil {
			return nodeAffinity, fmt.Errorf(`failed to parse "nodeAffinity.value" field: %w`, err)
		}
		nodeAffinity.Values = append(nodeAffinity.Values, valueString)
	}
	return nodeAffinity, nil
}

// getNamespace returns the namespace where the VM is created.
// VM is created in a dedicated namespace <cluster-id>
// which is the namespace where the machine-controller pod is running.
// Defaults to `kube-system`.
func getNamespace() string {
	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		// Useful especially for ci tests.
		ns = metav1.NamespaceSystem
	}
	return ns
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	sigClient, err := client.New(c.RestConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to get kubevirt client: %w", err)
	}

	virtualMachine := &kubevirtv1.VirtualMachine{}
	if err := sigClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: machine.Name}, virtualMachine); err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get VirtualMachine %s: %w", machine.Name, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	virtualMachineInstance := &kubevirtv1.VirtualMachineInstance{}
	if err := sigClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: machine.Name}, virtualMachineInstance); err != nil {
		if kerrors.IsNotFound(err) {
			return &kubeVirtServer{}, nil
		}

		return nil, err
	}

	// Deletion takes some time, so consider the VMI as deleted as soon as it has a DeletionTimestamp
	// because once the node got into status not ready its informers won't fire again
	// With the current approach we may run into a conflict when creating the VMI again, however this
	// results in the machine being reqeued
	if virtualMachineInstance.DeletionTimestamp != nil {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	if virtualMachineInstance.Status.Phase == kubevirtv1.Failed ||
		// The VMI enters phase succeeded if someone issues a kubectl
		// delete pod on the virt-launcher pod it runs in
		virtualMachineInstance.Status.Phase == kubevirtv1.Succeeded {
		// The pod got deleted, delete the VMI and return ErrNotFound so the VMI
		// will get recreated
		if err := sigClient.Delete(ctx, virtualMachineInstance); err != nil {
			return nil, fmt.Errorf("failed to delete failed VMI %s: %w", machine.Name, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	return &kubeVirtServer{vmi: *virtualMachineInstance}, nil
}

// We don't use the UID for kubevirt because the name of a VMI must stay stable
// in order for the node name to stay stable. The operator is responsible for ensuring
// there are no conflicts, e.G. by using one Namespace per Kubevirt user cluster.
func (p *provider) MigrateUID(_ context.Context, _ *clusterv1alpha1.Machine, _ types.UID) error {
	return nil
}

func (p *provider) Validate(ctx context.Context, spec clusterv1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	// If VMIPreset is specified, skip CPU and Memory validation.
	if c.FlavorName == "" {
		if _, err := parseResources(c.CPUs, c.Memory); err != nil {
			return err
		}
	}

	sigClient, err := client.New(c.RestConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to get kubevirt client: %w", err)
	}
	if _, ok := supportedOS[pc.OperatingSystem]; !ok {
		return fmt.Errorf("invalid/not supported operating system specified %q: %w", pc.OperatingSystem, providerconfigtypes.ErrOSNotSupported)
	}
	if c.DNSPolicy == corev1.DNSNone {
		if c.DNSConfig == nil || len(c.DNSConfig.Nameservers) == 0 {
			return fmt.Errorf("dns config must be specified when dns policy is None")
		}
	}
	// Check if we can reach the API of the target cluster.
	vmi := &kubevirtv1.VirtualMachineInstance{}
	if err := sigClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: "not-expected-to-exist"}, vmi); err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("failed to request VirtualMachineInstances: %w", err)
	}

	return nil
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %w", err)
	}
	cc := kubevirttypes.CloudConfig{
		Kubeconfig: c.Kubeconfig,
	}
	ccs, err := cc.String()

	return ccs, string(providerconfigtypes.CloudProviderExternal), err
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["cpus"] = c.CPUs
		labels["memoryMIB"] = c.Memory
		labels["osImage"] = c.OsImage.URL
	}

	return labels, err
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	// We add the timestamp because the secret name must be different when we recreate the VMI
	// because its pod got deleted
	// The secret has an ownerRef on the VMI so garbace collection will take care of cleaning up.
	terminationGracePeriodSeconds := int64(30)
	userDataSecretName := fmt.Sprintf("userdata-%s-%s", machine.Name, strconv.Itoa(int(time.Now().Unix())))

	resourceRequirements := kubevirtv1.ResourceRequirements{}
	labels := map[string]string{"kubevirt.io/vm": machine.Name}
	// Add a common label to all VirtualMachines spawned by the same MachineDeployment (= MachineDeployment name).
	if mdName, err := controllerutil.GetMachineDeploymentNameForMachine(ctx, machine, data.Client); err == nil {
		labels[machineDeploymentLabelKey] = mdName
	}

	sigClient, err := client.New(c.RestConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to get kubevirt client: %w", err)
	}

	// Add VMIPreset label if specified
	if c.FlavorName != "" {
		vmiPreset := kubevirtv1.VirtualMachineInstancePreset{}
		if err := sigClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.FlavorName}, &vmiPreset); err != nil {
			return nil, err
		}
		for key, val := range vmiPreset.Spec.Selector.MatchLabels {
			labels[key] = val
		}
	} else {
		requestsAndLimits, err := parseResources(c.CPUs, c.Memory)
		if err != nil {
			return nil, err
		}
		resourceRequirements.Requests = *requestsAndLimits
		resourceRequirements.Limits = *requestsAndLimits
	}

	var (
		dataVolumeName = machine.Name
		annotations    map[string]string
	)

	if pc.OperatingSystem == providerconfigtypes.OperatingSystemFlatcar {
		annotations = map[string]string{
			"kubevirt.io/ignitiondata": userdata,
		}
	}

	// we need this check until this issue is resolved:
	// https://github.com/kubevirt/containerized-data-importer/issues/895
	if len(dataVolumeName) > 63 {
		return nil, fmt.Errorf("dataVolumeName size %v, is bigger than 63 characters", len(dataVolumeName))
	}

	defaultBridgeNetwork, err := defaultBridgeNetwork()
	if err != nil {
		return nil, fmt.Errorf("could not compute a random MAC address")
	}

	virtualMachine := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machine.Name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Running: utilpointer.BoolPtr(true),
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						*kubevirtv1.DefaultPodNetwork(),
					},
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Disks:      getVMDisks(c),
							Interfaces: []kubevirtv1.Interface{*defaultBridgeNetwork},
						},
						Resources: resourceRequirements,
					},
					Affinity:                      getAffinity(c, machineDeploymentLabelKey, labels[machineDeploymentLabelKey]),
					TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					Volumes:                       getVMVolumes(c, dataVolumeName, userDataSecretName),
					DNSPolicy:                     c.DNSPolicy,
					DNSConfig:                     c.DNSConfig,
				},
			},
			DataVolumeTemplates: getDataVolumeTemplates(c, dataVolumeName),
		},
	}

	if err := sigClient.Create(ctx, virtualMachine); err != nil {
		return nil, fmt.Errorf("failed to create vmi: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            userDataSecretName,
			Namespace:       virtualMachine.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(virtualMachine, kubevirtv1.VirtualMachineGroupVersionKind)},
		},
		Data: map[string][]byte{"userdata": []byte(userdata)},
	}
	if err := sigClient.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("failed to create secret for userdata: %w", err)
	}
	return &kubeVirtServer{}, nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	sigClient, err := client.New(c.RestConfig, client.Options{})
	if err != nil {
		return false, fmt.Errorf("failed to get kubevirt client: %w", err)
	}

	vm := &kubevirtv1.VirtualMachine{}
	if err := sigClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: machine.Name}, vm); err != nil {
		if !kerrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get VirtualMachineInstance %s: %w", machine.Name, err)
		}
		// VMI is gone
		return true, nil
	}

	return false, sigClient.Delete(ctx, vm)
}

func parseResources(cpus, memory string) (*corev1.ResourceList, error) {
	memoryResource, err := resource.ParseQuantity(memory)
	if err != nil {
		return nil, fmt.Errorf("failed to parse memory requests: %w", err)
	}
	cpuResource, err := resource.ParseQuantity(cpus)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cpu request: %w", err)
	}
	return &corev1.ResourceList{
		corev1.ResourceMemory: memoryResource,
		corev1.ResourceCPU:    cpuResource,
	}, nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

func dnsPolicy(policy string) (corev1.DNSPolicy, error) {
	switch policy {
	case string(corev1.DNSClusterFirstWithHostNet):
		return corev1.DNSClusterFirstWithHostNet, nil
	case string(corev1.DNSClusterFirst):
		return corev1.DNSClusterFirst, nil
	case string(corev1.DNSDefault):
		return corev1.DNSDefault, nil
	case string(corev1.DNSNone):
		return corev1.DNSNone, nil
	}

	return "", fmt.Errorf("unknown dns policy: %s", policy)
}

func getVMDisks(config *Config) []kubevirtv1.Disk {
	disks := []kubevirtv1.Disk{
		{
			Name:       "datavolumedisk",
			DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
		},
		{
			Name:       "cloudinitdisk",
			DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
		},
	}
	for i := range config.SecondaryDisks {
		disks = append(disks, kubevirtv1.Disk{
			Name:       "secondarydisk" + strconv.Itoa(i),
			DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
		})
	}
	return disks
}

func defaultBridgeNetwork() (*kubevirtv1.Interface, error) {
	defaultBridgeNetwork := kubevirtv1.DefaultBridgeNetworkInterface()
	mac, err := netutil.GenerateRandMAC()
	if err != nil {
		return nil, err
	}
	defaultBridgeNetwork.MacAddress = mac.String()
	return defaultBridgeNetwork, nil
}

func getVMVolumes(config *Config, dataVolumeName string, userDataSecretName string) []kubevirtv1.Volume {
	volumes := []kubevirtv1.Volume{
		{
			Name: "datavolumedisk",
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: dataVolumeName,
				},
			},
		},
		{
			Name: "cloudinitdisk",
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
					UserDataSecretRef: &corev1.LocalObjectReference{
						Name: userDataSecretName,
					},
				},
			},
		},
	}
	for i := range config.SecondaryDisks {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: "secondarydisk" + strconv.Itoa(i),
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: "secondarydisk" + strconv.Itoa(i),
				}},
		})
	}
	return volumes
}

func getDataVolumeTemplates(config *Config, dataVolumeName string) []kubevirtv1.DataVolumeTemplateSpec {
	dataVolumeSource := getDataVolumeSource(config.OsImage)
	pvcRequest := corev1.ResourceList{corev1.ResourceStorage: config.PVCSize}
	dataVolumeTemplates := []kubevirtv1.DataVolumeTemplateSpec{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: dataVolumeName,
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				PVC: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: utilpointer.StringPtr(config.StorageClassName),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: pvcRequest,
					},
				},
				Source: dataVolumeSource,
			},
		},
	}
	for i, sd := range config.SecondaryDisks {
		dataVolumeTemplates = append(dataVolumeTemplates, kubevirtv1.DataVolumeTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name: "secondarydisk" + strconv.Itoa(i),
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				PVC: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: utilpointer.StringPtr(sd.StorageClassName),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: sd.Size},
					},
				},
				Source: dataVolumeSource,
			},
		})
	}
	return dataVolumeTemplates
}

// getDataVolumeSource returns DataVolumeSource, HTTP or PVC.
func getDataVolumeSource(osImage OSImage) *cdiv1beta1.DataVolumeSource {
	dataVolumeSource := &cdiv1beta1.DataVolumeSource{}
	if osImage.URL != "" {
		dataVolumeSource.HTTP = &cdiv1beta1.DataVolumeSourceHTTP{URL: osImage.URL}
	} else if osImage.DataVolumeName != "" {
		if nameSpaceAndName := strings.Split(osImage.DataVolumeName, "/"); len(nameSpaceAndName) >= 2 {
			dataVolumeSource.PVC = &cdiv1beta1.DataVolumeSourcePVC{
				Namespace: nameSpaceAndName[0],
				Name:      nameSpaceAndName[1],
			}
		}
	}
	return dataVolumeSource
}

func getAffinity(config *Config, matchKey, matchValue string) *corev1.Affinity {
	affinity := &corev1.Affinity{}

	// PodAffinity
	switch config.PodAffinityPreset {
	case softAffinityType:
		affinity.PodAffinity = &corev1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: hostnameWeightedAffinityTerm(matchKey, matchValue),
		}
	case hardAffinityType:
		affinity.PodAffinity = &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: hostnameAffinityTerm(matchKey, matchValue),
		}
	}

	// PodAntiAffinity
	switch config.PodAntiAffinityPreset {
	case softAffinityType:
		affinity.PodAntiAffinity = &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: hostnameWeightedAffinityTerm(matchKey, matchValue),
		}
	case hardAffinityType:
		affinity.PodAntiAffinity = &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: hostnameAffinityTerm(matchKey, matchValue),
		}
	}

	// NodeAffinity
	switch config.NodeAffinityPreset.Type {
	case softAffinityType:
		affinity.NodeAffinity = &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Weight: 1,
					Preference: corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      config.NodeAffinityPreset.Key,
								Values:   config.NodeAffinityPreset.Values,
								Operator: corev1.NodeSelectorOperator(metav1.LabelSelectorOpIn),
							},
						},
					},
				},
			},
		}
	case hardAffinityType:
		affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      config.NodeAffinityPreset.Key,
								Values:   config.NodeAffinityPreset.Values,
								Operator: corev1.NodeSelectorOperator(metav1.LabelSelectorOpIn),
							},
						},
					},
				},
			},
		}
	}

	return affinity
}

func hostnameWeightedAffinityTerm(matchKey, matchValue string) []corev1.WeightedPodAffinityTerm {
	return []corev1.WeightedPodAffinityTerm{
		{
			Weight: 1,
			PodAffinityTerm: corev1.PodAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						matchKey: matchValue,
					},
				},
				TopologyKey: topologyKeyHostname,
			},
		},
	}
}

func hostnameAffinityTerm(matchKey, matchValue string) []corev1.PodAffinityTerm {
	return []corev1.PodAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					matchKey: matchValue,
				},
			},
			TopologyKey: topologyKeyHostname,
		},
	}
}
