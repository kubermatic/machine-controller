package rhsm

import (
	"fmt"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	kuberneteshelper "github.com/kubermatic/machine-controller/pkg/kubernetes"
)

const (
	redhatSubscriptionFinalizer = "kubermatic.io/red-hat-subscription"
)

func AddRHELSubscriptionFinalizer(machine *v1alpha1.Machine, update types.MachineUpdater) error {
	if !kuberneteshelper.HasFinalizer(machine, redhatSubscriptionFinalizer) {
		if err := update(machine, func(m *v1alpha1.Machine) {
			machine.Finalizers = append(m.Finalizers, redhatSubscriptionFinalizer)
		}); err != nil {
			return fmt.Errorf("failed to add redhat subscription finalizer: %v", err)
		}
	}

	return nil
}

func RemoveRHELSubscriptionFinalizer(machine *v1alpha1.Machine, update types.MachineUpdater) error {
	if kuberneteshelper.HasFinalizer(machine, redhatSubscriptionFinalizer) {
		if err := update(machine, func(m *v1alpha1.Machine) {
			machine.Finalizers = kuberneteshelper.RemoveFinalizer(machine.Finalizers, redhatSubscriptionFinalizer)
		}); err != nil {
			return fmt.Errorf("failed to remove redhat subscription finalizer: %v", err)
		}
	}

	return nil
}
