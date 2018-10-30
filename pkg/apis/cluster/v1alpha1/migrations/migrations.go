package migrations

import (
	"fmt"
	"time"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/conversions"
	machinesv1alpha1clientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	"github.com/kubermatic/machine-controller/pkg/machines"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"github.com/golang/glog"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

func MigrateMachinesv1Alpha1MachineToClusterv1Alpha1MachineIfNecessary(
	kubeClient kubernetes.Interface,
	apiextClient apiextclient.Interface,
	clusterv1Alpha1Client clusterv1alpha1clientset.Interface,
	config *restclient.Config) error {

	_, err := apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(machines.CRDName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			glog.Infof("CRD %s not present, no migration needed", machines.CRDName)
			return nil
		}
		return fmt.Errorf("failed to get crds: %v", err)
	}

	_, err = apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get("machines.cluster.k8s.io", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error when checking for existence of 'machines.cluster.k8s.io' crd: %v", err)
	}

	machinesv1Alpha1MachineClient, err := machinesv1alpha1clientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create machinesv1alpha1clientset: %v", err)
	}

	if err = migrateMachines(kubeClient,
		machinesv1Alpha1MachineClient,
		clusterv1Alpha1Client); err != nil {
		return fmt.Errorf("failed to migrate machines: %v", err)
	}
	glog.Infof("Attempting to delete CRD %s", machines.CRDName)
	if err = apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(machines.CRDName, nil); err != nil {
		return fmt.Errorf("failed to delete machinesv1alpha1.machine crd: %v", err)
	}
	glog.Infof("Successfully deleted CRD %s", machines.CRDName)
	return nil
}

func migrateMachines(kubeClient kubernetes.Interface,
	machinesv1Alpha1MachineClient machinesv1alpha1clientset.Interface,
	clusterv1Alpha1Client clusterv1alpha1clientset.Interface) error {

	glog.Infof("Starting migration for machine.machines.k8s.io/v1alpha1 to machine.cluster.k8s.io/v1alpha1")

	// Get machinesv1Alpha1Machines
	glog.Infof("Getting existing machine.machines.k8s.io/v1alpha1 to migrate")
	machinesv1Alpha1Machines, err := machinesv1Alpha1MachineClient.MachineV1alpha1().Machines().List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list machinesV1Alpha1 machines: %v", err)
	}
	glog.Infof("Found %v machine.machines.k8s.io/v1alpha1", len(machinesv1Alpha1Machines.Items))

	// Convert the machine, create the new machine, delete the old one, wait for it to be absent
	// We do this in one loop to avoid ending up having all machines in  both the new and the old format if deletion
	// failes for whatever reason
	for _, machinesV1Alpha1Machine := range machinesv1Alpha1Machines.Items {
		glog.Infof("Starting migration for machine.machines.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
		convertedClusterv1alpha1Machine := &clusterv1alpha1.Machine{}
		err := conversions.Convert_MachinesV1alpha1Machine_To_ClusterV1alpha1Machine(&machinesV1Alpha1Machine,
			convertedClusterv1alpha1Machine)
		if err != nil {
			return fmt.Errorf("failed to convert machinesV1alpha1.machine to clusterV1alpha1.machine name=%s err=%v",
				machinesV1Alpha1Machine.Name, err)
		}
		convertedClusterv1alpha1Machine.Finalizers = append(convertedClusterv1alpha1Machine.Finalizers, machinecontroller.FinalizerDeleteNode)

		// Some providers need to update the provider instance to the new UID, we get the provider as early as possible
		// to not fail in a half-migrated state when the providerconfig is invalid
		providerConfig, err := providerconfig.GetConfig(convertedClusterv1alpha1Machine.Spec.ProviderConfig)
		if err != nil {
			return fmt.Errorf("failed to get provider config: %v", err)
		}
		skg := providerconfig.NewConfigVarResolver(kubeClient)
		prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
		if err != nil {
			return fmt.Errorf("failed to get cloud provider %q: %v", providerConfig.CloudProvider, err)
		}

		// We will set that to whats finally in the apisever, be that a created a clusterv1alpha1machine
		// or a preexisting one, because the migration got interrupted
		// It is required to set the ownerRef of the node
		var owningClusterV1Alpha1Machine *clusterv1alpha1.Machine

		// Do a get first to cover the case the new machine was already created but then something went wrong
		// If that is the case and the clusterv1alpha1machine != machinesv1alpha1machine we error out and the operator
		// has to manually delete either the new or the old machine
		glog.Infof("Checking if machine.cluster.k8s.io/v1alpha1 %s/%s already exists",
			convertedClusterv1alpha1Machine.Namespace, convertedClusterv1alpha1Machine.Name)
		existingClusterV1alpha1Machine, err := clusterv1Alpha1Client.ClusterV1alpha1().Machines(
			convertedClusterv1alpha1Machine.Namespace).Get(convertedClusterv1alpha1Machine.Name, metav1.GetOptions{})
		if err != nil {
			// Some random error occured
			if !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to check if converted machine %s already exists: %v", convertedClusterv1alpha1Machine.Name, err)
			}

			// ClusterV1alpha1Machine does not exist yet
			glog.Infof("Machine.cluster.k8s.io/v1alpha1 %s/%s does not yet exist, attempting to create it",
				convertedClusterv1alpha1Machine.Namespace, convertedClusterv1alpha1Machine.Name)
			owningClusterV1Alpha1Machine, err = clusterv1Alpha1Client.ClusterV1alpha1().Machines(convertedClusterv1alpha1Machine.Namespace).Create(convertedClusterv1alpha1Machine)
			if err != nil {
				return fmt.Errorf("failed to create clusterv1alpha1.machine %s: %v", convertedClusterv1alpha1Machine.Name, err)
			}
			glog.Infof("Successfully created machine.cluster.k8s.io/v1alpha1 %s/%s",
				convertedClusterv1alpha1Machine.Namespace, convertedClusterv1alpha1Machine.Name)
		} else {
			// ClusterV1alpha1Machine already exists
			if !equality.Semantic.DeepEqual(convertedClusterv1alpha1Machine.Spec, existingClusterV1alpha1Machine.Spec) {
				return fmt.Errorf("---manual intervention required!--- Spec of machines.v1alpha1.machine %s is not equal to clusterv1alpha1.machines %s/%s, delete either of them to allow migration to succeed",
					machinesV1Alpha1Machine.Name, convertedClusterv1alpha1Machine.Namespace, convertedClusterv1alpha1Machine.Name)
			}
			existingClusterV1alpha1Machine.Labels = convertedClusterv1alpha1Machine.Labels
			existingClusterV1alpha1Machine.Annotations = convertedClusterv1alpha1Machine.Annotations
			existingClusterV1alpha1Machine.Finalizers = convertedClusterv1alpha1Machine.Finalizers
			glog.Infof("Updating existing machine.cluster.k8s.io/v1alpha1 %s/%s",
				existingClusterV1alpha1Machine.Namespace, existingClusterV1alpha1Machine.Name)
			if owningClusterV1Alpha1Machine, err = clusterv1Alpha1Client.ClusterV1alpha1().Machines(existingClusterV1alpha1Machine.Namespace).Update(existingClusterV1alpha1Machine); err != nil {
				return fmt.Errorf("failed to update metadata of existing clusterV1Alpha1 machine: %v", err)
			}
			glog.Infof("Successfully updated existing machine.cluster.k8s.io/v1alpha1 %s/%s",
				existingClusterV1alpha1Machine.Namespace, existingClusterV1alpha1Machine.Name)
		}

		// We have to ensure there is an ownerRef to our clusterv1alpha1.Machine on the node if it exists
		// and that there is no ownerRef to the old machine anymore
		if err := ensureClusterV1Alpha1NodeOwnership(owningClusterV1Alpha1Machine, kubeClient); err != nil {
			return err
		}

		if sets.NewString(owningClusterV1Alpha1Machine.Finalizers...).Has(machinecontroller.FinalizerDeleteInstance) {
			glog.Infof("Attempting to update the UID at the cloud provider for machine.cluster.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
			newMachineWithOldUID := owningClusterV1Alpha1Machine.DeepCopy()
			newMachineWithOldUID.UID = machinesV1Alpha1Machine.UID
			if err := prov.MigrateUID(newMachineWithOldUID, owningClusterV1Alpha1Machine.UID); err != nil {
				return fmt.Errorf("running the provider migration for the UID failed: %v", err)
			}
			// Block until we can actually GET the instance with the new UID
			var isMigrated bool
			for i := 0; i < 100; i++ {
				if _, err := prov.Get(owningClusterV1Alpha1Machine); err == nil {
					isMigrated = true
					break
				}
				time.Sleep(10 * time.Second)
			}
			if !isMigrated {
				return fmt.Errorf("failed to GET instance for machine %s after UID migration", owningClusterV1Alpha1Machine.Name)
			}
			glog.Infof("Successfully updated the UID at the cloud provider for machine.cluster.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
		}

		// All went fine, we only have to clear the old machine now
		glog.Infof("Deleting machine.machines.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
		if err := deleteMachinesV1Alpha1Machine(&machinesV1Alpha1Machine, machinesv1Alpha1MachineClient); err != nil {
			return err
		}
		glog.Infof("Successfully deleted machine.machines.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
		glog.Infof("Successfully finished migration for machine.machines.k8s.io/v1alpha1 %s", machinesV1Alpha1Machine.Name)
	}

	glog.Infof("Successfully finished migration for machine.machines.k8s.io/v1alpha1 to machine.cluster.k8s.io/v1alpha1")
	return nil
}

func ensureClusterV1Alpha1NodeOwnership(machine *clusterv1alpha1.Machine, kubeClient kubernetes.Interface) error {
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}
	glog.Infof("Checking if node for machines.cluster.k8s.io/v1alpha1 %s/%s exists",
		machine.Namespace, machine.Name)
	nodeNameCandidates := []string{machine.Spec.Name}
	if machine.Status.NodeRef != nil {
		if machine.Status.NodeRef.Name != machine.Spec.Name {
			nodeNameCandidates = append(nodeNameCandidates, machine.Status.NodeRef.Name)
		}
	}

	for _, nodeName := range nodeNameCandidates {
		node, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				glog.Infof("No node for machines.cluster.k8s.io/v1alpha1 %s/%s found",
					machine.Namespace, machine.Name)
				continue
			}
			return fmt.Errorf("Failed to get node %s for machine %s: %v",
				machine.Spec.Name, machine.Name, err)
		}

		glog.Infof("Found node for machines.cluster.k8s.io/v1alpha1 %s/%s: %s, removing its ownerRef and adding NodeOwnerLabel",
			node.Name, machine.Namespace, machine.Name)
		nodeLabels := node.Labels
		nodeLabels[machinecontroller.NodeOwnerLabelName] = string(machine.UID)
		// We retry this because nodes get frequently updated so there is a reasonable chance this may fail
		if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, err := kubeClient.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			// Clear all OwnerReferences as a safety measure
			node.OwnerReferences = nil
			node.Labels = nodeLabels
			_, err = kubeClient.CoreV1().Nodes().Update(node)
			return err
		}); err != nil {
			return fmt.Errorf("failed to update OwnerLabel on node %s: %v", node.Name, err)
		}
		glog.Infof("Successfully removed ownerRef and added NodeOwnerLabelName to node %s for machines.cluster.k8s.io/v1alpha1 %s/%s",
			node.Name, machine.Namespace, machine.Name)
	}

	return nil
}

func deleteMachinesV1Alpha1Machine(machine *machinesv1alpha1.Machine,
	machineClient machinesv1alpha1clientset.Interface) error {

	machine.Finalizers = []string{}
	if _, err := machineClient.MachineV1alpha1().Machines().Update(machine); err != nil {
		return fmt.Errorf("failed to update machinesv1alpha1.machine %s after removing finalizer: %v", machine.Name, err)
	}
	if err := machineClient.MachineV1alpha1().Machines().Delete(machine.Name, nil); err != nil {
		return fmt.Errorf("failed to delete machine %s: %v", machine.Name, err)
	}

	if err := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		return isMachinesV1Alpha1MachineDeleted(machine.Name, machineClient)
	}); err != nil {
		return fmt.Errorf("failed to wait for machine %s to be deleted: %v", machine.Name, err)
	}

	return nil
}

func isMachinesV1Alpha1MachineDeleted(name string, client machinesv1alpha1clientset.Interface) (bool, error) {
	if _, err := client.MachineV1alpha1().Machines().Get(name, metav1.GetOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
