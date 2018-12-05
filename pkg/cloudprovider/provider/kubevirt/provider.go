package kubevirt

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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
	Config        providerconfig.ConfigVarString `json:"config"`
	CPUs          providerconfig.ConfigVarString `json:"cpus"`
	Memory        providerconfig.ConfigVarString `json:"memory"`
	RegistryImage providerconfig.ConfigVarString `json:"registryImage"`
	Namespace     providerconfig.ConfigVarString `json:"namespace"`
}

type Config struct {
	Config        rest.Config
	CPUs          string
	Memory        string
	RegistryImage string
	Namespace     string
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
	config := Config{}
	configString, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Config, "KUBEVIRT_KUBECONFIG")
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "config" field: %v`, err)
	}
	config.CPUs, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.CPUs)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "cpus" field: %v`, err)
	}
	config.Memory, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Memory)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "memory" field: %v`, err)
	}
	config.RegistryImage, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.RegistryImage)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "registryImage" field: %v`, err)
	}
	config.Namespace, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf(`failed to get value of "namespace" field: %v`, err)
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(configString))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode kubeconfig: %v", err)
	}
	config.Config = *restConfig

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

	virtualMachineInstance, err := client.VirtualMachineInstance(c.Namespace).Get(machine.Name, &metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get VirtualMachineInstance %s: %v", machine.Name, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound

	}
	// Deletion takes some time, so consider the VMI as deleted as soon as it has a DeletionTimestamp
	// because once the node got into status not ready its informers wont fire again
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
		if err := client.VirtualMachine(c.Namespace).Delete(machine.Name, &metav1.DeleteOptions{}); err != nil {
			return nil, fmt.Errorf("failed to delete failed VMI %s: %v", machine.Name, err)
		}
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	return &kubeVirtServer{vmi: *virtualMachineInstance}, nil
}

// We don't use the UID for kubevirt because the name of a VMI must stay stable
// in order for the node name to stay stable. The operator is responsible for ensuring
// there are no conflicts, e.G. by using one Namespace per Kubevirt user cluster
func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	return nil
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}
	if _, err := parseResources(c.CPUs, c.Memory); err != nil {
		return err
	}
	if pc.OperatingSystem == providerconfig.OperatingSystemCoreos {
		return fmt.Errorf("CoreOS is not supported")
	}
	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt client: %v", err)
	}
	// Check if we can reach the API of the target cluster
	_, err = client.VirtualMachineInstance(c.Namespace).Get("not-expected-to-exist", &metav1.GetOptions{})
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
		labels["cpus"] = c.CPUs
		labels["memoryMIB"] = c.Memory
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

	// We add the timestamp because the secret name must be different when we recreate the VMI
	// because its pod got deleted
	// The secret has an ownerRef on the VMI so garbace collection will take care of cleaning up
	terminationGracePeriodSeconds := int64(30)
	userdataSecretName := fmt.Sprintf("userdata-%s-%s", machine.Name, strconv.Itoa(int(time.Now().Unix())))
	requestsAndLimits, err := parseResources(c.CPUs, c.Memory)
	if err != nil {
		return nil, err
	}
	virtualMachineInstance := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machine.Name,
			Namespace: c.Namespace,
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					Disks: []kubevirtv1.Disk{
						{
							Name:       "registryDisk",
							VolumeName: "registryvolume",
							DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
						},
						{
							Name:       "cloudinitdisk",
							VolumeName: "cloudinitvolume",
							DiskDevice: kubevirtv1.DiskDevice{Disk: &kubevirtv1.DiskTarget{Bus: "virtio"}},
						},
					},
				},
				Resources: kubevirtv1.ResourceRequirements{
					Requests: *requestsAndLimits,
					Limits:   *requestsAndLimits,
				},
			},
			// Must be set because of https://github.com/kubevirt/kubevirt/issues/178
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			Volumes: []kubevirtv1.Volume{
				{
					Name: "registryvolume",
					VolumeSource: kubevirtv1.VolumeSource{
						RegistryDisk: &kubevirtv1.RegistryDiskSource{Image: c.RegistryImage},
					},
				},
				{
					Name: "cloudinitvolume",
					VolumeSource: kubevirtv1.VolumeSource{
						CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
							UserDataSecretRef: &corev1.LocalObjectReference{
								Name: userdataSecretName,
							},
						},
					},
				},
			},
		},
	}

	client, err := kubecli.GetKubevirtClientFromRESTConfig(&c.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubevirt client: %v", err)
	}

	createdVMI, err := client.VirtualMachineInstance(virtualMachineInstance.Namespace).
		Create(virtualMachineInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create vmi: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            userdataSecretName,
			Namespace:       createdVMI.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(createdVMI, kubevirtv1.VirtualMachineInstanceGroupVersionKind)},
		},
		Data: map[string][]byte{"userdata": []byte(userdata)},
	}
	_, err = client.CoreV1().Secrets(secret.Namespace).Create(secret)
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
	_, err = client.VirtualMachineInstance(c.Namespace).Get(machine.Name, &metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get VirtualMachineInstance %s: %v", machine.Name, err)
		}
		// VMI is gone
		return true, nil
	}

	return false, client.VirtualMachineInstance(c.Namespace).Delete(machine.Name, &metav1.DeleteOptions{})
}

func parseResources(cpus, memory string) (*corev1.ResourceList, error) {
	memoryResource, err := resource.ParseQuantity(memory)
	if err != nil {
		return nil, fmt.Errorf("failed to parse memory requests: %v", err)
	}
	cpuResource, err := resource.ParseQuantity(cpus)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cpu request: %v", err)
	}
	return &corev1.ResourceList{
		corev1.ResourceMemory: memoryResource,
		corev1.ResourceCPU:    cpuResource,
	}, nil
}
