package cloud

import (
	"crypto/rsa"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
)

// Provider exposed all required functions to interact with a cloud provider
type Provider interface {
	Validate(machinespec v1alpha1.MachineSpec) error
	Get(machine *v1alpha1.Machine) (instance.Instance, error)
	GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error)
	Create(machine *v1alpha1.Machine, userdata string, key rsa.PublicKey) (instance.Instance, error)
	Delete(machine *v1alpha1.Machine) error
}
