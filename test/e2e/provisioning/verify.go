package provisioning

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/golang/glog"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

const (
	machineReadyCheckPeriod = 15 * time.Second
)

func verifyCreateMachineFails(kubeConfig, manifestPath string, parameters []string, _ time.Duration) error {
	_, clusterClient, machine, err := prepareMachine(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}
	if _, err := clusterClient.ClusterV1alpha1().Machines(machine.Namespace).Create(machine); err != nil {
		return nil
	}
	return fmt.Errorf("expected create of Machine %s to fail but succeeded", machine.Name)
}

func verifyCreateAndDelete(kubeConfig, manifestPath string, parameters []string, timeout time.Duration) error {

	kubeClient, clusterClient, machineDeployment, err := prepareMachineDeployment(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}

	machineDeployment, err = createAndAssure(machineDeployment, clusterClient, kubeClient, timeout)
	if err != nil {
		return fmt.Errorf("failed to verify creation of node for MachineDeployment: %v", err)
	}

	err = deleteAndAssure(machineDeployment, clusterClient, kubeClient, timeout)
	if err != nil {
		return fmt.Errorf("Failed to verify if a machine/node has been created/deleted, due to: \n%v", err)
	}

	glog.Infof("Successfully finished test for MachineDeployment %s", machineDeployment.Name)
	return nil
}

func prepareMachineDeployment(kubeConfig, manifestPath string, parameters []string) (kubernetes.Interface, clientset.Interface, *v1alpha1.MachineDeployment, error) {

	kubeClient, clusterClient, manifest, err := prepare(kubeConfig, manifestPath, parameters)
	if err != nil {
		return nil, nil, nil, err
	}
	newMachineDeployment := &v1alpha1.MachineDeployment{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	err = manifestDecoder.Decode(newMachineDeployment)
	if err != nil {
		return nil, nil, nil, err
	}
	// Enforce the kube-system namespace, otherwise cleanup wont work
	newMachineDeployment.Namespace = "kube-system"

	return kubeClient, clusterClient, newMachineDeployment, nil
}

func prepareMachine(kubeConfig, manifestPath string, parameters []string) (kubernetes.Interface, clientset.Interface, *v1alpha1.Machine, error) {

	kubeClient, clusterClient, manifest, err := prepare(kubeConfig, manifestPath, parameters)
	if err != nil {
		return nil, nil, nil, err
	}
	newMachine := &v1alpha1.Machine{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	err = manifestDecoder.Decode(newMachine)
	if err != nil {
		return nil, nil, nil, err
	}
	// Enforce the kube-system namespace, otherwise cleanup wont work
	newMachine.Namespace = "kube-system"

	return kubeClient, clusterClient, newMachine, nil
}

func prepare(kubeConfig, manifestPath string, parameters []string) (kubernetes.Interface,
	clientset.Interface, string, error) {
	if len(manifestPath) == 0 || len(kubeConfig) == 0 {
		return nil, nil, "", fmt.Errorf("kubeconfig and manifest path must be defined")
	}

	// init kube related stuff
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Error building kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Error building kubernetes clientset: %v", err)
	}
	clusterClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Error building example clientset: %v", err)
	}

	// prepare the manifest
	manifest, err := readAndModifyManifest(manifestPath, parameters)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to prepare the manifest, due to: %v", err)
	}

	return kubeClient, clusterClient, manifest, nil
}

func createAndAssure(machineDeployment *v1alpha1.MachineDeployment,
	clusterClient clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) (*v1alpha1.MachineDeployment, error) {
	// we expect that no node for machine exists in the cluster
	err := assureNodeForMachineDeployment(machineDeployment, kubeClient, clusterClient, false)
	if err != nil {
		return nil, fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	glog.Infof("creating a new \"%s\" MachineDeployment\n", machineDeployment.Name)
	machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Create(machineDeployment)
	if err != nil {
		return nil, err
	}
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		pollErr := assureNodeForMachineDeployment(machineDeployment, kubeClient, clusterClient, true)
		if pollErr == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed waiting for MachineDeployment %s to get a node: %v", machineDeployment.Name, err)
	}
	glog.Infof("Found a node for MachineDeployment %s", machineDeployment.Name)

	glog.Infof("Waiting for node of MachineDeployment %s to become ready", machineDeployment.Name)
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		machines, pollErr := getMatchingMachines(machineDeployment, clusterClient)
		if pollErr != nil || len(machines) < 1 {
			return false, nil
		}
		for _, machine := range machines {
			hasReadyNode, pollErr := hasMachineReadyNode(&machine, kubeClient, clusterClient)
			if err != nil {
				return false, pollErr
			}
			if hasReadyNode {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed waiting for MachineDeployment %s to get a node in ready state: %v", machineDeployment.Name, err)
	}
	return machineDeployment, nil
}

func hasMachineReadyNode(machine *v1alpha1.Machine, kubeClient kubernetes.Interface, clusterClient clientset.Interface) (bool, error) {
	nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list nodes: %v", err)
	}
	for _, node := range nodes.Items {
		if isNodeForMachine(&node, machine) {
			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func deleteAndAssure(machineDeployment *v1alpha1.MachineDeployment,
	clusterClient clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) error {
	glog.Infof("Starting to clean up MachineDeployment %s", machineDeployment.Name)

	// We first scale down to 0, because once the machineSets are deleted we can not
	// match machines anymore and we do want to verify not only the node is gone but also
	// the instance at the cloud provider
	if err := updateMachineDeployment(machineDeployment, clusterClient, func(md *v1alpha1.MachineDeployment) {
		md.Spec.Replicas = getInt32Ptr(0)
	}); err != nil {
		return fmt.Errorf("failed to update replicas of MachineDeployment %s: %v", machineDeployment.Name, err)
	}

	// Ensure machines are gone
	if err := wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		ownedMachines, err := getMatchingMachines(machineDeployment, clusterClient)
		if err != nil {
			return false, err
		}
		if len(ownedMachines) != 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for machines of MachineDeployment %s to be deleted: %v", machineDeployment.Name, err)
	}

	glog.V(2).Infof("Deleting MachineDeployment %s", machineDeployment.Name)
	err := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Delete(machineDeployment.Name, nil)
	if err != nil {
		return fmt.Errorf("unable to remove MachineDeployment %s, due to %v", machineDeployment.Name, err)
	}
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		_, errGetMachineDeployment := clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Get(machineDeployment.Name, metav1.GetOptions{})
		if errGetMachineDeployment != nil && kerrors.IsNotFound(errGetMachineDeployment) {
			return true, nil
		}
		return false, errGetMachineDeployment
	})
	return nil
}

// assureNodeForMachineDeployment according to shouldExists parameter check if a node for machine exists in the system or not
// this method examines OwnerReference of each node.
func assureNodeForMachineDeployment(machineDeployment *v1alpha1.MachineDeployment, kubeClient kubernetes.Interface, clusterClient clientset.Interface, shouldExists bool) error {
	nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	machines, err := getMatchingMachines(machineDeployment, clusterClient)
	if err != nil {
		return fmt.Errorf("failed to list Machines: %v", err)
	}

	nodeForMachineExists := false
	for _, machine := range machines {
		for _, node := range nodes.Items {
			if isNodeForMachine(&node, &machine) {
				nodeForMachineExists = true
				break
			}
		}
	}

	if shouldExists != nodeForMachineExists {
		return fmt.Errorf("expeced a node in the system to exists=%v but have found a node in the current cluster=%v", shouldExists, nodeForMachineExists)
	}
	return nil
}

func isNodeForMachine(node *corev1.Node, machine *v1alpha1.Machine) bool {
	// This gets called before the Objects are persisted in the API
	// which means UI will be emppy for machine
	if string(machine.UID) == "" {
		return false
	}
	return node.Labels[machinecontroller.NodeOwnerLabelName] == string(machine.UID)
}

func readAndModifyManifest(pathToManifest string, keyValuePairs []string) (string, error) {
	contentRaw, err := ioutil.ReadFile(pathToManifest)
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf("%s", contentRaw)

	for _, keyValuePair := range keyValuePairs {
		// stopping on the first encountered match allows to read base64 encoded values
		kv := strings.SplitN(keyValuePair, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("the given key value pair = %v is incorrect, the correct form is key=value", keyValuePair)
		}
		content = strings.Replace(content, kv[0], kv[1], -1)
	}

	return content, nil
}

// getMatchingMachines returns all machines that are owned by the passed machineDeployment
func getMatchingMachines(machineDeployment *v1alpha1.MachineDeployment, clusterClient clientset.Interface) ([]v1alpha1.Machine, error) {
	matchingMachineSets, err := getMachingMachineSets(machineDeployment, clusterClient)
	if err != nil {
		return nil, err
	}
	glog.V(2).Infof("Found %v matching MachineSets for %s", len(matchingMachineSets), machineDeployment.Name)
	var matchingMachines []v1alpha1.Machine
	for _, machineSet := range matchingMachineSets {
		machinesForMachineSet, err := getMatchingMachinesForMachineset(&machineSet, clusterClient)
		if err != nil {
			return nil, fmt.Errorf("failed to get matching Machines for MachineSet %s: %v", machineSet.Name, err)
		}
		matchingMachines = append(matchingMachines, machinesForMachineSet...)
	}
	glog.V(2).Infof("Found %v matching Machines for MachineDeployment %s", len(matchingMachines), machineDeployment.Name)
	return matchingMachines, nil
}

func getMatchingMachinesForMachineset(machineSet *v1alpha1.MachineSet, clusterClient clientset.Interface) ([]v1alpha1.Machine, error) {
	allMachines, err := clusterClient.ClusterV1alpha1().Machines(machineSet.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Machines: %v", err)
	}
	var matchingMachines []v1alpha1.Machine
	for _, machine := range allMachines.Items {
		if metav1.GetControllerOf(&machine) != nil && metav1.IsControlledBy(&machine, machineSet) {
			matchingMachines = append(matchingMachines, machine)
		}
	}
	return matchingMachines, nil
}

// getMachingMachineSets returns all machineSets that are owned by the passed machineDeployment
func getMachingMachineSets(machineDeployment *v1alpha1.MachineDeployment, clusterClient clientset.Interface) ([]v1alpha1.MachineSet, error) {
	// Ensure we actually have an object from the KubeAPI and not just the result of the yaml parsing, as the latter
	// can not be the owner of anything due to missing UID
	if machineDeployment.ResourceVersion == "" {
		var err error
		machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Get(machineDeployment.Name, metav1.GetOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get MachineDeployment %s: %v", machineDeployment.Name, err)
			}
			return nil, nil
		}
	}
	allMachineSets, err := clusterClient.ClusterV1alpha1().MachineSets(machineDeployment.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MachineSets: %v", err)
	}
	var matchingMachineSets []v1alpha1.MachineSet
	for _, machineSet := range allMachineSets.Items {
		if metav1.GetControllerOf(&machineSet) != nil && metav1.IsControlledBy(&machineSet, machineDeployment) {
			matchingMachineSets = append(matchingMachineSets, machineSet)
		}
	}
	return matchingMachineSets, nil
}

func getInt32Ptr(i int32) *int32 {
	return &i
}

func updateMachineDeployment(md *v1alpha1.MachineDeployment, clusterClient clientset.Interface, modify func(*v1alpha1.MachineDeployment)) error {
	// Store Namespace and Name here because after an error md will be nil
	name := md.Name
	namespace := md.Namespace

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var err error
		md, err = clusterClient.ClusterV1alpha1().MachineDeployments(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		modify(md)
		md, err = clusterClient.ClusterV1alpha1().MachineDeployments(namespace).Update(md)
		return err
	})
}
