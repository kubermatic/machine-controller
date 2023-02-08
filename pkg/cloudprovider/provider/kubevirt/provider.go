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
	if err := cdiv1beta1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add cdiv1beta1 to scheme: %v", err)
	}
}

type imageSource string

const (
	// topologyKeyHostname defines the topology key for the node hostname.
	topologyKeyHostname = "kubernetes.io/hostname"
	// machineDeploymentLabelKey defines the label key used to contains as value the MachineDeployment name
	// which machine comes from.
	machineDeploymentLabelKey = "md"
	// httpSource defines the http source type for VM Disk Image.
	httpSource imageSource = "http"
	// pvcSource defines the pvc source type for VM Disk Image.
	pvcSource imageSource = "pvc"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a Kubevirt provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Kubeconfig                string
	ClusterName               string
	RestConfig                *rest.Config
	DNSConfig                 *corev1.PodDNSConfig
	DNSPolicy                 corev1.DNSPolicy
	CPUs                      string
	Memory                    string
	Namespace                 string
	OSImageSource             *cdiv1beta1.DataVolumeSource
	StorageClassName          string
	PVCSize                   resource.Quantity
	Instancetype              *kubevirtv1.InstancetypeMatcher
	Preference                *kubevirtv1.PreferenceMatcher
	SecondaryDisks            []SecondaryDisks
	NodeAffinityPreset        NodeAffinityPreset
	TopologySpreadConstraints []corev1.TopologySpreadConstraint
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
	Name             string
	Size             resource.Quantity
	StorageClassName string
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

func (k *kubeVirtServer) ProviderID() string {
	return "kubevirt://" + k.vmi.Name
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

	config.ClusterName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ClusterName)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "clusterName" field: %w`, err)
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

	config.OSImageSource, err = p.parseOSImageSource(rawConfig.VirtualMachine.Template.PrimaryDisk, config.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "osImageSource" field: %w`, err)
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

	// Instancetype and Preference
	config.Instancetype = rawConfig.VirtualMachine.Instancetype
	config.Preference = rawConfig.VirtualMachine.Preference

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
	for i, sd := range rawConfig.VirtualMachine.Template.SecondaryDisks {
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
			Name:             fmt.Sprintf("secondarydisk%d", i),
			Size:             pvc,
			StorageClassName: scString,
		})
	}

	config.NodeAffinityPreset, err = p.parseNodeAffinityPreset(rawConfig.Affinity.NodeAffinityPreset)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "nodeAffinityPreset" field: %w`, err)
	}
	config.TopologySpreadConstraints, err = p.parseTopologySpreadConstraint(rawConfig.TopologySpreadConstraints)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to parse "topologySpreadConstraints" field: %w`, err)
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
	nodeAffinity.Values = make([]string, 0, len(nodeAffinityPreset.Values))
	for _, v := range nodeAffinityPreset.Values {
		valueString, err := p.configVarResolver.GetConfigVarStringValue(v)
		if err != nil {
			return nodeAffinity, fmt.Errorf(`failed to parse "nodeAffinity.value" field: %w`, err)
		}
		nodeAffinity.Values = append(nodeAffinity.Values, valueString)
	}
	return nodeAffinity, nil
}

func (p *provider) parseTopologySpreadConstraint(topologyConstraints []kubevirttypes.TopologySpreadConstraint) ([]corev1.TopologySpreadConstraint, error) {
	parsedTopologyConstraints := make([]corev1.TopologySpreadConstraint, 0, len(topologyConstraints))
	for _, constraint := range topologyConstraints {
		maxSkewString, err := p.configVarResolver.GetConfigVarStringValue(constraint.MaxSkew)
		if err != nil {
			return nil, fmt.Errorf(`failed to parse "topologySpreadConstraint.maxSkew" field: %w`, err)
		}
		maxSkew, err := strconv.ParseInt(maxSkewString, 10, 32)
		if err != nil {
			return nil, fmt.Errorf(`failed to parse "topologySpreadConstraint.maxSkew" field: %w`, err)
		}
		topologyKey, err := p.configVarResolver.GetConfigVarStringValue(constraint.TopologyKey)
		if err != nil {
			return nil, fmt.Errorf(`failed to parse "topologySpreadConstraint.topologyKey" field: %w`, err)
		}
		whenUnsatisfiable, err := p.configVarResolver.GetConfigVarStringValue(constraint.WhenUnsatisfiable)
		if err != nil {
			return nil, fmt.Errorf(`failed to parse "topologySpreadConstraint.whenUnsatisfiable" field: %w`, err)
		}
		parsedTopologyConstraints = append(parsedTopologyConstraints, corev1.TopologySpreadConstraint{
			MaxSkew:           int32(maxSkew),
			TopologyKey:       topologyKey,
			WhenUnsatisfiable: corev1.UnsatisfiableConstraintAction(whenUnsatisfiable),
		})
	}
	return parsedTopologyConstraints, nil
}

func (p *provider) parseOSImageSource(primaryDisk kubevirttypes.PrimaryDisk, namespace string) (*cdiv1beta1.DataVolumeSource, error) {
	osImage, err := p.configVarResolver.GetConfigVarStringValue(primaryDisk.OsImage)
	if err != nil {
		return nil, fmt.Errorf(`failed to get value of "primaryDisk.osImage" field: %w`, err)
	}
	osImageSource, err := p.configVarResolver.GetConfigVarStringValue(primaryDisk.Source)
	if err != nil {
		return nil, fmt.Errorf(`failed to get value of "primaryDisk.source" field: %w`, err)
	}
	switch imageSource(osImageSource) {
	case httpSource:
		return &cdiv1beta1.DataVolumeSource{HTTP: &cdiv1beta1.DataVolumeSourceHTTP{URL: osImage}}, nil
	case pvcSource:
		if namespaceAndName := strings.Split(osImage, "/"); len(namespaceAndName) >= 2 {
			return &cdiv1beta1.DataVolumeSource{PVC: &cdiv1beta1.DataVolumeSourcePVC{Name: namespaceAndName[1], Namespace: namespaceAndName[0]}}, nil
		}
		return &cdiv1beta1.DataVolumeSource{PVC: &cdiv1beta1.DataVolumeSourcePVC{Name: osImage, Namespace: namespace}}, nil
	default:
		// handle old API for backward compatibility.
		if _, err = url.ParseRequestURI(osImage); err == nil {
			return &cdiv1beta1.DataVolumeSource{HTTP: &cdiv1beta1.DataVolumeSourceHTTP{URL: osImage}}, nil
		}
		if namespaceAndName := strings.Split(osImage, "/"); len(namespaceAndName) >= 2 {
			return &cdiv1beta1.DataVolumeSource{PVC: &cdiv1beta1.DataVolumeSourcePVC{Name: namespaceAndName[1], Namespace: namespaceAndName[0]}}, nil
		}
		return &cdiv1beta1.DataVolumeSource{PVC: &cdiv1beta1.DataVolumeSourcePVC{Name: osImage, Namespace: namespace}}, nil
	}
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
	// If instancetype is specified, skip CPU and Memory validation.
	// Values will come from instancetype.
	if c.Instancetype == nil {
		if _, err := parseResources(c.CPUs, c.Memory); err != nil {
			return err
		}
	}

	sigClient, err := client.New(c.RestConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to get kubevirt client: %w", err)
	}
	if _, ok := kubevirttypes.SupportedOS[pc.OperatingSystem]; !ok {
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
		Namespace: c.Namespace,
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
		if c.OSImageSource.HTTP != nil {
			labels["osImage"] = c.OSImageSource.HTTP.URL
		} else if c.OSImageSource.PVC != nil {
			labels["osImage"] = c.OSImageSource.PVC.Name
		}
	}

	return labels, err
}

type machineDeploymentNameGetter func() (string, error)

func machineDeploymentNameAndRevisionForMachineGetter(ctx context.Context, machine *clusterv1alpha1.Machine, c client.Client) machineDeploymentNameGetter {
	mdName, _, err := controllerutil.GetMachineDeploymentNameAndRevisionForMachine(ctx, machine, c)
	return func() (string, error) {
		return mdName, err
	}
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
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

	userDataSecretName := fmt.Sprintf("userdata-%s-%s", machine.Name, strconv.Itoa(int(time.Now().Unix())))

	virtualMachine, err := p.newVirtualMachine(ctx, c, pc, machine, userDataSecretName, userdata,
		machineDeploymentNameAndRevisionForMachineGetter(ctx, machine, data.Client), randomMacAddressGetter, sigClient)
	if err != nil {
		return nil, fmt.Errorf("could not create a VirtualMachine manifest %w", err)
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

func (p *provider) newVirtualMachine(ctx context.Context, c *Config, pc *providerconfigtypes.Config, machine *clusterv1alpha1.Machine,
	userdataSecretName, userdata string, mdNameGetter machineDeploymentNameGetter, macAddressGetter macAddressGetter, sigClient client.Client) (*kubevirtv1.VirtualMachine, error) {
	// We add the timestamp because the secret name must be different when we recreate the VMI
	// because its pod got deleted
	// The secret has an ownerRef on the VMI so garbace collection will take care of cleaning up.
	terminationGracePeriodSeconds := int64(30)

	evictionStrategy := kubevirtv1.EvictionStrategyExternal

	resourceRequirements := kubevirtv1.ResourceRequirements{}
	labels := map[string]string{"kubevirt.io/vm": machine.Name}
	//Add a common label to all VirtualMachines spawned by the same MachineDeployment (= MachineDeployment name).
	if mdName, err := mdNameGetter(); err == nil {
		labels[machineDeploymentLabelKey] = mdName
	}

	// if no instancetype, resources are from config.
	if c.Instancetype == nil {
		requestsAndLimits, err := parseResources(c.CPUs, c.Memory)
		if err != nil {
			return nil, err
		}
		resourceRequirements.Requests = *requestsAndLimits
		resourceRequirements.Limits = *requestsAndLimits
	}

	// Add cluster labels
	labels["cluster.x-k8s.io/cluster-name"] = c.ClusterName
	labels["cluster.x-k8s.io/role"] = "worker"

	var (
		dataVolumeName = machine.Name
		annotations    map[string]string
	)
	// Add machineName as prefix to secondaryDisks.
	addPrefixToSecondaryDisk(c.SecondaryDisks, dataVolumeName)

	if pc.OperatingSystem == providerconfigtypes.OperatingSystemFlatcar {
		annotations = map[string]string{
			"kubevirt.io/ignitiondata": userdata,
		}
	}

	defaultBridgeNetwork, err := defaultBridgeNetwork(macAddressGetter)
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
			Running:      utilpointer.Bool(true),
			Instancetype: c.Instancetype,
			Preference:   c.Preference,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					EvictionStrategy: &evictionStrategy,
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
					Volumes:                       getVMVolumes(c, dataVolumeName, userdataSecretName),
					DNSPolicy:                     c.DNSPolicy,
					DNSConfig:                     c.DNSConfig,
					TopologySpreadConstraints:     getTopologySpreadConstraints(c, map[string]string{machineDeploymentLabelKey: labels[machineDeploymentLabelKey]}),
				},
			},
			DataVolumeTemplates: getDataVolumeTemplates(c, dataVolumeName),
		},
	}
	return virtualMachine, nil
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
	for _, sd := range config.SecondaryDisks {
		disks = append(disks, kubevirtv1.Disk{
			Name:       sd.Name,
			DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
		})
	}
	return disks
}

type macAddressGetter func() (string, error)

func randomMacAddressGetter() (string, error) {
	mac, err := netutil.GenerateRandMAC()
	if err != nil {
		return "", err
	}
	return mac.String(), nil
}

func defaultBridgeNetwork(macAddressGetter macAddressGetter) (*kubevirtv1.Interface, error) {
	defaultBridgeNetwork := kubevirtv1.DefaultBridgeNetworkInterface()
	mac, err := macAddressGetter()
	if err != nil {
		return nil, err
	}
	defaultBridgeNetwork.MacAddress = mac
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
	for _, sd := range config.SecondaryDisks {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: sd.Name,
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: sd.Name,
				}},
		})
	}
	return volumes
}

func getDataVolumeTemplates(config *Config, dataVolumeName string) []kubevirtv1.DataVolumeTemplateSpec {
	pvcRequest := corev1.ResourceList{corev1.ResourceStorage: config.PVCSize}
	dataVolumeTemplates := []kubevirtv1.DataVolumeTemplateSpec{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: dataVolumeName,
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				PVC: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: utilpointer.String(config.StorageClassName),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: pvcRequest,
					},
				},
				Source: config.OSImageSource,
			},
		},
	}
	for _, sd := range config.SecondaryDisks {
		dataVolumeTemplates = append(dataVolumeTemplates, kubevirtv1.DataVolumeTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name: sd.Name,
			},
			Spec: cdiv1beta1.DataVolumeSpec{
				PVC: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: utilpointer.String(sd.StorageClassName),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: sd.Size},
					},
				},
				Source: config.OSImageSource,
			},
		})
	}
	return dataVolumeTemplates
}

func getAffinity(config *Config, matchKey, matchValue string) *corev1.Affinity {
	affinity := &corev1.Affinity{}

	expressions := []corev1.NodeSelectorRequirement{
		{
			Key:      config.NodeAffinityPreset.Key,
			Operator: corev1.NodeSelectorOperator(metav1.LabelSelectorOpExists),
		},
	}

	// change the operator if any values were passed for node affinity matching
	if len(config.NodeAffinityPreset.Values) > 0 {
		expressions[0].Operator = corev1.NodeSelectorOperator(metav1.LabelSelectorOpIn)
		expressions[0].Values = config.NodeAffinityPreset.Values
	}

	// NodeAffinity
	switch config.NodeAffinityPreset.Type {
	case softAffinityType:
		affinity.NodeAffinity = &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Weight: 1,
					Preference: corev1.NodeSelectorTerm{
						MatchExpressions: expressions,
					},
				},
			},
		}
	case hardAffinityType:
		affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: expressions,
					},
				},
			},
		}
	}

	return affinity
}

func addPrefixToSecondaryDisk(secondaryDisks []SecondaryDisks, prefix string) {
	for i := range secondaryDisks {
		secondaryDisks[i].Name = fmt.Sprintf("%s-%s", prefix, secondaryDisks[i].Name)
	}
}

func getTopologySpreadConstraints(config *Config, matchLabels map[string]string) []corev1.TopologySpreadConstraint {
	if len(config.TopologySpreadConstraints) != 0 {
		for i := range config.TopologySpreadConstraints {
			config.TopologySpreadConstraints[i].LabelSelector = &metav1.LabelSelector{MatchLabels: matchLabels}
		}
		return config.TopologySpreadConstraints
	}
	// Return default TopologySpreadConstraint
	return []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       topologyKeyHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: matchLabels},
		},
	}
}
