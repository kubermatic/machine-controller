package kubevirt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Azure/go-autorest/autorest/to"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/kubecli"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a Kubevirt provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloud.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type RawConfig struct {
	Config        string `json:"config"`
	CPUs          int32  `json:"cpus"`
	MemoryMiB     int64  `json:"memoryMIB"`
	RegistryImage string `json:"registryImage"`
}

type Config struct {
	Config        rest.Config
	CPUs          int32
	MemoryMiB     int64
	RegistryImage string
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

func (k *kubeVirtServer) Addresses() []string {
	var addresses []string
	for _, kvInterface := range k.vmi.Status.Interfaces {
		addresses = append(addresses, kvInterface.IP)
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

func (p *provider) getConfig(s v1alpha1.ProviderConfig) (*Config, *providerconfig.Config, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}

	//TODO: Use RawConfig to allow resolving via secretReg/ConfigMapRef
	rawConfig := RawConfig{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, err
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(rawConfig.Config))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode kubeconfig: %v", err)
	}
	config := Config{
		Config:        *restConfig,
		CPUs:          rawConfig.CPUs,
		MemoryMiB:     rawConfig.MemoryMiB,
		RegistryImage: rawConfig.RegistryImage,
	}

	return &config, &pconfig, nil
}

func (p *provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubevirt client: %v", err)
	}

	//TODO: Should the namespace be configurable?
	virtualMachineInstance, err := client.VirtualMachineInstance(metav1.NamespaceSystem).Get(string(machine.UID), &metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get VirtualMachineInstance %s: %v", machine.UID, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	if virtualMachineInstance.Status.Phase == kubevirtv1.Failed {
		// The pod got deleted, delete the VMI and return ErrNotFound so the VMI
		// will get recreated
		if err := client.VirtualMachine(metav1.NamespaceSystem).Delete(string(machine.UID), &metav1.DeleteOptions{}); err != nil {
			return nil, fmt.Errorf("failed to delete failed VMI %s: %v", machine.UID, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	return &kubeVirtServer{vmi: *virtualMachineInstance}, nil
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	return errors.New("not implemented")
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	c, _, err := p.getConfig(spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}
	if c.CPUs < 1 {
		return errors.New("CPUs must be 1 or greater")
	}
	if c.MemoryMiB < 1 {
		return errors.New("MemoryMiB must be 1 or greater")
	}
	if _, err := parseResources(c.CPUs, c.MemoryMiB); err != nil {
		return err
	}
	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt client: %v", err)
	}
	// Check if we can reach the API of the target cluster
	_, err = client.VirtualMachineInstance(metav1.NamespaceSystem).Get("not-expected-to-exist", &metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("failed to request VirtualMachineInstances: %v", err)
	}

	return nil
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err == nil {
		labels["cpus"] = strconv.Itoa(int(c.CPUs))
		labels["memoryMIB"] = strconv.Itoa(int(c.MemoryMiB))
		labels["registryImage"] = c.RegistryImage
	}

	return labels, err
}

func (p *provider) Create(machine *v1alpha1.Machine, _ *cloud.MachineCreateDeleteData, userdata string) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	virtualMachineInstance := &kubevirtv1.VirtualMachineInstance{}
	virtualMachineInstance.Name = string(machine.UID)
	virtualMachineInstance.Namespace = metav1.NamespaceSystem
	virtualMachineInstance.Spec.Domain.CPU = &kubevirtv1.CPU{Cores: uint32(c.CPUs)}
	// Must be set because of https://github.com/kubevirt/kubevirt/issues/1780
	virtualMachineInstance.Spec.TerminationGracePeriodSeconds = to.Int64Ptr(30)
	var disks []kubevirtv1.Disk
	disks = append(disks, kubevirtv1.Disk{
		Name:       "registryDisk",
		VolumeName: "registryvolume",
		DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
	})
	disks = append(disks, kubevirtv1.Disk{
		Name:       "cloudinitdisk",
		VolumeName: "cloudinitvolume",
		DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
	})
	virtualMachineInstance.Spec.Domain.Devices.Disks = disks
	requestsAndLimits, err := parseResources(c.CPUs, c.MemoryMiB)
	if err != nil {
		return nil, err
	}
	virtualMachineInstance.Spec.Domain.Resources.Requests = *requestsAndLimits
	virtualMachineInstance.Spec.Domain.Resources.Limits = *requestsAndLimits
	var volumes []kubevirtv1.Volume
	volumes = append(volumes, kubevirtv1.Volume{
		Name: "registryvolume",
		VolumeSource: kubevirtv1.VolumeSource{
			RegistryDisk: &kubevirtv1.RegistryDiskSource{Image: c.RegistryImage},
		},
	})

	userdataSecretName := fmt.Sprintf("machine-controller-userdata-%s", machine.UID)
	volumes = append(volumes, kubevirtv1.Volume{
		Name: "cloudinitvolume",
		VolumeSource: kubevirtv1.VolumeSource{
			CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
				UserDataSecretRef: &corev1.LocalObjectReference{
					Name: userdataSecretName,
				},
			},
		},
	})
	virtualMachineInstance.Spec.Volumes = volumes

	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubevirt client: %v", err)
	}

	createdVMI, err := client.VirtualMachineInstance(virtualMachineInstance.Namespace).
		Create(virtualMachineInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create vmi: %v", err)
	}

	secret := &corev1.Secret{}
	secret.Name = userdataSecretName
	secret.Namespace = createdVMI.Namespace
	ownerRef := *metav1.NewControllerRef(createdVMI, kubevirtv1.VirtualMachineInstanceGroupVersionKind)
	secret.OwnerReferences = append(secret.OwnerReferences, ownerRef)
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data["userdata"] = []byte(userdata)
	_, err = client.CoreV1().Secrets(secret.Namespace).Create(secret)
	// TODO: What happens when the pod for the vmi dies and we re-create the VMI?
	// A secret with the given name may still exist
	if err != nil {
		return nil, fmt.Errorf("failed to create secret for userdata: %v", err)
	}
	return &kubeVirtServer{vmi: *createdVMI}, nil

}

func (p *provider) Cleanup(machine *v1alpha1.Machine, _ *cloud.MachineCreateDeleteData) (bool, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return false, fmt.Errorf("failed to get kubevirt client: %v", err)
	}
	_, err = client.VirtualMachineInstance(metav1.NamespaceSystem).Get(string(machine.UID), &metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get VirtualMachineInstance %s: %v", string(machine.UID), err)
		}
		// VMI is gone
		return true, nil
	}

	return false, client.VirtualMachineInstance(metav1.NamespaceSystem).Delete(string(machine.UID), &metav1.DeleteOptions{})
}

func parseResources(cpus int32, memoryMiB int64) (*corev1.ResourceList, error) {
	memoryResource, err := resource.ParseQuantity(fmt.Sprintf("%vM", memoryMiB))
	if err != nil {
		return nil, fmt.Errorf("failed to parse memory requests: %v", err)
	}
	cpuResource, err := resource.ParseQuantity(strconv.Itoa(int(cpus)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse cpu request: %v", err)
	}
	return &corev1.ResourceList{
		corev1.ResourceMemory: memoryResource,
		corev1.ResourceCPU:    cpuResource,
	}, nil
}
