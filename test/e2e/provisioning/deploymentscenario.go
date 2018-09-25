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

	kubeClient, clusterClient, machineDeployment, err := prepare(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}
	// This test inherently relies on replicas being one so we enforce that
	machineDeployment.Spec.Replicas = getInt32Ptr(1)

	machineDeployment, err = createAndAssure(machineDeployment, clusterClient, kubeClient, timeout)
	if err != nil {
		return fmt.Errorf("failed to verify creation of node for machineDeployment %s: %v", machineDeployment.Name, err)
	}

	machineDeployment.Spec.Template.Labels["testUpdate"] = "true"
	machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Update(machineDeployment)
	if err != nil {
		return fmt.Errorf("failed to update machineDeployment %s after modiying it: %v", machineDeployment.Name, err)
	}

	glog.Infof("Waiting for second machineSet to appear after updating machineDeployment %s", machineDeployment.Name)
	var machineSets []v1alpha1.MachineSet
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
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
	glog.Infof("Found second machineSet for machineDeployment %s!", machineDeployment.Name)

	glog.Infof("Waiting for new machineSets node to appear")
	var newestMachineSet, oldMachineSet v1alpha1.MachineSet
	if machineSets[0].CreationTimestamp.Before(&machineSets[1].CreationTimestamp) {
		newestMachineSet = machineSets[1]
		oldMachineSet = machineSets[0]
	} else {
		newestMachineSet = machineSets[0]
		oldMachineSet = machineSets[1]
	}
	var machines []v1alpha1.Machine
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
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
	glog.Infof("New machineSet %s appeared with %v machines", newestMachineSet.Name, len(machines))

	glog.Infof("Waiting for new machineSet %s to get a ready node", newestMachineSet.Name)
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
		return hasMachineReadyNode(&machines[0], kubeClient, clusterClient)
	}); err != nil {
		return err
	}
	glog.Infof("Found ready node for machineSet %s", newestMachineSet.Name)

	glog.Infof("Waiting for old machineSet %s to be scaled down and have no associated machines",
		oldMachineSet.Name)
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
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
	glog.Infof("Old machineSet %s got scaled down and has no associated machiens anymore", oldMachineSet.Name)

	glog.Infof("Setting replicas of machineDeployment %s to 0 and waiting until it has no associated machines", machineDeployment.Name)
	machineDeployment.Spec.Replicas = getInt32Ptr(0)
	machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Update(machineDeployment)
	if err != nil {
		return fmt.Errorf("failed to update replicas of machineDeployment %s: %v", machineDeployment.Name, err)
	}
	glog.Infof("Successfully set replicas of machineDeployment %s to 0", machineDeployment.Name)

	glog.Infof("Waiting for machineDeployment %s to not have any associated machines", machineDeployment.Name)
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
		machines, err := getMatchingMachines(machineDeployment, clusterClient)
		return len(machines) == 0, err
	}); err != nil {
		return err
	}
	glog.Infof("Successfully waited for machineDeployment %s to not have any associated machines", machineDeployment.Name)

	glog.Infof("Deleting machineDeployment %s and waiting for it to disappear", machineDeployment.Name)
	if err := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Delete(machineDeployment.Name, nil); err != nil {
		return fmt.Errorf("failed to delete machineDeployment %s: %v", machineDeployment.Name, err)
	}
	if err := wait.Poll(1*time.Second, timeout, func() (bool, error) {
		_, err := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Get(machineDeployment.Name, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		return err
	}
	glog.Infof("Successfully deleted machineDeployment %s!", machineDeployment.Name)
	return nil
}
