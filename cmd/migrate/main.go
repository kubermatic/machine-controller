package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"

	conversions "github.com/kubermatic/machine-controller/pkg/api/conversions"
	downstreammachineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	"github.com/kubermatic/machine-controller/pkg/controller"
	downstreammachines "github.com/kubermatic/machine-controller/pkg/machines"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"

	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

var (
	kubeconfig string
	masterURL  string
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	apiExtClient := apiextclient.NewForConfigOrDie(cfg)
	clusterv1alpha1Client := clusterv1alpha1clientset.NewForConfigOrDie(cfg)
	kubeClient := kubernetes.NewForConfigOrDie(cfg)

	if err = migrateIfNecesary(kubeClient, apiExtClient, clusterv1alpha1Client, cfg); err != nil {
		glog.Fatalf("Failed to migrate: %v", err)
	}
}

func migrateIfNecesary(kubeClient kubernetes.Interface,
	apiextClient apiextclient.Interface,
	clusterv1alpha1Client clusterv1alpha1clientset.Interface,
	config *restclient.Config) error {

	_, err := apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(downstreammachines.CRDName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get crds: %v", err)
	}

	downstreamClient, err := downstreammachineclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create downstream machine client: %v", err)
	}

	if err = migrateMachines(kubeClient, downstreamClient, clusterv1alpha1Client); err != nil {
		return fmt.Errorf("failed to migrate machines: %v", err)
	}
	if err = apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(downstreammachines.CRDName, nil); err != nil {
		return fmt.Errorf("failed to delete downstream crd: %v", err)
	}

	return nil
}

func migrateMachines(kubeClient kubernetes.Interface,
	downstreamClient downstreammachineclientset.Interface,
	clusterv1alpha1Client clusterv1alpha1clientset.Interface) error {

	// Get downstreamMachines
	downstreamMachines, err := downstreamClient.MachineV1alpha1().Machines().List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list downstream machines: %v", err)
	}

	// Convert the machine, create the new machine, delete the old one, wait for it to be absent
	// We do this in one loop to avoid ending up having all machines in  both the new and the old format if deletion
	// failes for whatever reason
	for _, downstreamMachine := range downstreamMachines.Items {
		convertedClusterv1alpha1Machine, err := conversions.ConvertV1alpha1DownStreamMachineToV1alpha1ClusterMachine(downstreamMachine)
		if err != nil {
			return fmt.Errorf("failed to convert machine %s: %v", downstreamMachine.Name, err)
		}

		createdClusterV1alpha1Machine, err := clusterv1alpha1Client.ClusterV1alpha1().Machines(convertedClusterv1alpha1Machine.Namespace).Create(convertedClusterv1alpha1Machine)
		if err != nil {
			return fmt.Errorf("failed to create clusterv1alpha1.machine %s: %v", convertedClusterv1alpha1Machine.Name, err)
		}

		node, err := kubeClient.CoreV1().Nodes().Get(convertedClusterv1alpha1Machine.Spec.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("Failed to get node %s for machine %s: %v", convertedClusterv1alpha1Machine.Spec.Name, convertedClusterv1alpha1Machine.Name, err)
		}
		for idx, ownerRef := range node.OwnerReferences {
			if ownerRef.UID == downstreamMachine.UID {
				node.OwnerReferences = append(node.OwnerReferences[:idx], node.OwnerReferences[idx+1:]...)
				break
			}
		}
		newOwnerRef := node.OwnerReferences
		gv := clusterv1alpha1.SchemeGroupVersion
		newOwnerRef = append(newOwnerRef, *metav1.NewControllerRef(createdClusterV1alpha1Machine, gv.WithKind("Machine")))

		// We retry this because nodes get frequently updated so there is a reasonable chance this may fail
		if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, err := kubeClient.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			node.OwnerReferences = newOwnerRef
			if _, err := kubeClient.CoreV1().Nodes().Update(node); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to update OwnerRef on node %s: %v", node.Name, err)
		}

		finalizers := sets.NewString(downstreamMachine.Finalizers...)
		finalizers.Delete(controller.FinalizerDeleteInstance)
		downstreamMachine.Finalizers = finalizers.List()
		if _, err := downstreamClient.MachineV1alpha1().Machines().Update(&downstreamMachine); err != nil {
			return fmt.Errorf("failed to update downstream machine %s after removing finalizer: %v", convertedClusterv1alpha1Machine.Name, err)
		}
		if err := downstreamClient.MachineV1alpha1().Machines().Delete(convertedClusterv1alpha1Machine.Name, nil); err != nil {
			return fmt.Errorf("failed to delete machine %s: %v", convertedClusterv1alpha1Machine.Name, err)
		}

		if err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
			return isDownstreamMachineDeleted(convertedClusterv1alpha1Machine.Name, downstreamClient)
		}); err != nil {
			return fmt.Errorf("failed to wait for machine %s to be deleted: %v", convertedClusterv1alpha1Machine.Name, err)
		}
	}
	return nil
}

func isDownstreamMachineDeleted(name string, client downstreammachineclientset.Interface) (bool, error) {
	if _, err := client.MachineV1alpha1().Machines().Get(name, metav1.GetOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
