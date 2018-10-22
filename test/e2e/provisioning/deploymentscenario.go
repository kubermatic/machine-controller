package provisioning

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func verifyCreateUpdateAndDelete(kubeConfig, manifestPath string, parameters []string, timeout time.Duration) error {

	kubeClient, clusterClient, machineDeployment, err := prepareMachineDeployment(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}
	// This test inherently relies on replicas being one so we enforce that
	machineDeployment.Spec.Replicas = getInt32Ptr(1)

	machineDeployment, err = createAndAssure(machineDeployment, clusterClient, kubeClient, timeout)
	if err != nil {
		return fmt.Errorf("failed to verify creation of node for MachineDeployment: %v", err)
	}

	if err := updateMachineDeployment(machineDeployment, clusterClient, func(md *v1alpha1.MachineDeployment) {
		md.Spec.Template.Labels["testUpdate"] = "true"
	}); err != nil {
		return fmt.Errorf("failed to update MachineDeployment %s after modifying it: %v", machineDeployment.Name, err)
	}

	glog.Infof("Waiting for second MachineSet to appear after updating MachineDeployment %s", machineDeployment.Name)
	var machineSets []v1alpha1.MachineSet
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		machineSets, err = getMachingMachineSets(machineDeployment, clusterClient)
		if err != nil {
			return false, err
		}
		if len(machineSets) != 2 {
			return false, err
		}
		for _, machineSet := range machineSets {
			if *machineSet.Spec.Replicas != int32(1) {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return err
	}
	glog.Infof("Found second MachineSet for MachineDeployment %s!", machineDeployment.Name)

	glog.Infof("Waiting for new MachineSets node to appear")
	var newestMachineSet, oldMachineSet v1alpha1.MachineSet
	if machineSets[0].CreationTimestamp.Before(&machineSets[1].CreationTimestamp) {
		newestMachineSet = machineSets[1]
		oldMachineSet = machineSets[0]
	} else {
		newestMachineSet = machineSets[0]
		oldMachineSet = machineSets[1]
	}
	var machines []v1alpha1.Machine
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		machines, err = getMatchingMachinesForMachineset(&newestMachineSet, clusterClient)
		if err != nil {
			return false, err
		}
		if len(machines) != 1 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return err
	}
	glog.Infof("New MachineSet %s appeared with %v machines", newestMachineSet.Name, len(machines))

	glog.Infof("Waiting for new MachineSet %s to get a ready node", newestMachineSet.Name)
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		return hasMachineReadyNode(&machines[0], kubeClient, clusterClient)
	}); err != nil {
		return err
	}
	glog.Infof("Found ready node for MachineSet %s", newestMachineSet.Name)

	glog.Infof("Waiting for old MachineSet %s to be scaled down and have no associated machines",
		oldMachineSet.Name)
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		machineSet, err := clusterClient.ClusterV1alpha1().MachineSets(oldMachineSet.Namespace).Get(oldMachineSet.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if *machineSet.Spec.Replicas != int32(0) {
			return false, nil
		}
		machines, err := getMatchingMachinesForMachineset(machineSet, clusterClient)
		if err != nil {
			return false, err
		}
		return len(machines) == 0, nil
	}); err != nil {
		return err
	}
	glog.Infof("Old MachineSet %s got scaled down and has no associated machines anymore", oldMachineSet.Name)

	glog.Infof("Setting replicas of MachineDeployment %s to 0 and waiting until it has no associated machines", machineDeployment.Name)
	if err := updateMachineDeployment(machineDeployment, clusterClient, func(md *v1alpha1.MachineDeployment) {
		md.Spec.Replicas = getInt32Ptr(0)
	}); err != nil {
		return fmt.Errorf("failed to update replicas of MachineDeployment %s: %v", machineDeployment.Name, err)
	}
	glog.Infof("Successfully set replicas of MachineDeployment %s to 0", machineDeployment.Name)

	glog.Infof("Waiting for MachineDeployment %s to not have any associated machines", machineDeployment.Name)
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		machines, err := getMatchingMachines(machineDeployment, clusterClient)
		return len(machines) == 0, err
	}); err != nil {
		return err
	}
	glog.Infof("Successfully waited for MachineDeployment %s to not have any associated machines", machineDeployment.Name)

	glog.Infof("Deleting MachineDeployment %s and waiting for it to disappear", machineDeployment.Name)
	if err := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Delete(machineDeployment.Name, nil); err != nil {
		return fmt.Errorf("failed to delete MachineDeployment %s: %v", machineDeployment.Name, err)
	}
	if err := wait.Poll(5*time.Second, timeout, func() (bool, error) {
		_, err := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Get(machineDeployment.Name, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		return err
	}
	glog.Infof("Successfully deleted MachineDeployment %s!", machineDeployment.Name)
	return nil
}
