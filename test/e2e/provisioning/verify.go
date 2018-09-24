package provisioning

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	"k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

const (
	machineReadyCheckPeriod = 15 * time.Second
	tempDir                 = "/tmp"
)

func verify(kubeConfig, manifestPath string, parameters []string, timeout time.Duration) error {

	if len(manifestPath) == 0 || len(kubeConfig) == 0 {
		return fmt.Errorf("kubeconfig and manifest path must be defined")
	}

	// init kube related stuff
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return fmt.Errorf("Error building kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("Error building kubernetes clientset: %v", err)
	}
	clusterClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("Error building example clientset: %v", err)
	}

	// prepare the manifest
	manifests, err := readAndModifyManifest(manifestPath, parameters)
	if err != nil {
		return fmt.Errorf("failed to prepare the manifest, due to: %v", err)
	}

	manifestsList := strings.Split(manifests, "\n---\n")
	for _, manifest := range manifestsList {
		if manifest == "" {
			continue
		}
		if strings.Contains(manifest, "kind: MachineDeployment") {
			newMachineDeployment := &v1alpha1.MachineDeployment{}
			manifestReader := strings.NewReader(manifest)
			manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
			err = manifestDecoder.Decode(newMachineDeployment)
			if err != nil {
				return err
			}
			// Enforce the kube-system namespace, otherwise cleanup wont work
			newMachineDeployment.Namespace = "kube-system"

			err = createAndAssure(newMachineDeployment, clusterClient, kubeClient, timeout)
			if err != nil {
				return err
			}

			err = deleteAndAssure(newMachineDeployment, clusterClient, kubeClient, timeout)
			if err != nil {
				return fmt.Errorf("Failed to verify if a machine/node has been created/deleted, due to: \n%v", err)
			}

			msg := "all good, successfully verified that a machine/node has been created and then deleted"
			glog.Infoln(msg)
		} else {
			// Be pragmatic
			glog.Infof("Trying to apply additional manifest...")
			err = kubectlApply(kubeConfig, manifest)
			if err != nil {
				return fmt.Errorf("error applying manifest: '%v'", err)
			}
			glog.Infof("Successfully applied additional manifest!")
		}
	}

	return nil
}

func kubectlApply(kubecfgPath, manifest string) error {
	file, err := ioutil.TempFile(tempDir, "")
	if err != nil {
		return err
	}
	_, err = file.WriteString(manifest)
	if err != nil {
		return err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	filePath := filepath.Join(tempDir, fileInfo.Name())
	glog.Infof("Wrote temporary manifest file to '%s'", filePath)

	cmdSlice := []string{"kubectl", "--kubeconfig", kubecfgPath, "apply", "-f", filePath}
	command := exec.Command(cmdSlice[0], cmdSlice[1:]...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error executing command '%s': '%v'\nOutput:\n%s", strings.Join(cmdSlice, " "), err, string(output))
	}

	return nil
}

func createAndAssure(machineDeployment *v1alpha1.MachineDeployment,
	clusterClient clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) error {
	// we expect that no node for machine exists in the cluster
	err := assureNodeForMachineDeployment(machineDeployment, kubeClient, clusterClient, false)
	if err != nil {
		return fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	glog.Infof("creating a new \"%s\" machineDeployment\n", machineDeployment.Name)
	machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Create(machineDeployment)
	if err != nil {
		return err
	}
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		pollErr := assureNodeForMachineDeployment(machineDeployment, kubeClient, clusterClient, true)
		if pollErr == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("falied to created the new machineSet, err: %v", err)
	}

	glog.Infof("waiting for status = %s to come \n", v1.NodeReady)
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		machines, pollErr := getMatchingMachines(machineDeployment, clusterClient)
		if pollErr != nil || len(machines) < 1 {
			return false, nil
		}
		nodes, pollErr := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if pollErr != nil {
			return false, nil
		}

		for _, machine := range machines {
			for _, node := range nodes.Items {
				if isNodeForMachine(&node, &machine) {
					for _, condition := range node.Status.Conditions {
						if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
							return true, nil
						}
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("falied to created the new machine, err = %v", err)
	}
	return nil
}

func deleteAndAssure(machineDeployment *v1alpha1.MachineDeployment,
	clusterClient clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) error {
	glog.Infof("Starting to clean up machineDeployment %s", machineDeployment.Name)

	// We first scale down to 0, because once the machineSets are deleted we can not
	// match machines anymore and we do want to verify not only the node is gone but also
	// the instance at the cloud provider
	machineSets, err := getMachingMachineSets(machineDeployment, clusterClient)
	if err != nil {
		return err
	}
	for _, machineSet := range machineSets {
		machineSet.Spec.Replicas = getInt32Ptr(0)
		_, err = clusterClient.ClusterV1alpha1().MachineSets(machineSet.Namespace).Update(&machineSet)
		if err != nil {
			return fmt.Errorf("unable to set update replicas of machineset %s: %v", machineSet.Name, err)
		}
	}

	// Ensure machines are gone
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		ownedMachines, err := getMatchingMachines(machineDeployment, clusterClient)
		if err != nil {
			return false, err
		}
		if len(ownedMachines) != 0 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for machines to be deleted, err = %v", err)
	}

	glog.V(2).Infof("Deleting machineDeployment %s", machineDeployment.Name)
	err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Delete(machineDeployment.Name, nil)
	if err != nil {
		return fmt.Errorf("unable to remove machine deployment %s, due to %v", machineDeployment.Name, err)
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
		return fmt.Errorf("failed to list machines: %v", err)
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

func isNodeForMachine(node *v1.Node, machine *v1alpha1.Machine) bool {
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
	glog.V(2).Infof("Found %v matching machineSets for %s", len(matchingMachineSets), machineDeployment.Name)
	allMachines, err := clusterClient.ClusterV1alpha1().Machines(machineDeployment.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list machines: %v", err)
	}
	var matchingMachines []v1alpha1.Machine
	for _, machineSet := range matchingMachineSets {
		for _, machine := range allMachines.Items {
			if metav1.GetControllerOf(&machine) != nil && metav1.IsControlledBy(&machine, &machineSet) {
				matchingMachines = append(matchingMachines, machine)
			}
		}
	}
	glog.V(2).Infof("Found %v matching machines for %s", len(matchingMachines), machineDeployment.Name)
	return matchingMachines, nil
}

// getMachingMachineSets returns all machineSets that are owned by the passed machineDeployment
func getMachingMachineSets(machineDeployment *v1alpha1.MachineDeployment, clusterClient clientset.Interface) ([]v1alpha1.MachineSet, error) {
	// Ensure we actually have an object from the KubeAPI and not just the result of the yaml parsing, as the latter
	// can not be the owner of anything due to missing UID
	if machineDeployment.ResourceVersion == "" {
		var err error
		machineDeployment, err = clusterClient.ClusterV1alpha1().MachineDeployments(machineDeployment.Namespace).Get(machineDeployment.Name)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to get machineDeployment %s: %v", machineDeployment.Name, err)
			}
			return nil, nil
		}
	}
	allMachineSets, err := clusterClient.ClusterV1alpha1().MachineSets(machineDeployment.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list machineSets: %v", err)
	}
	var matchingMachineSets []v1alpha1.MachineSet
	for _, machineSet := range allMachineSets.Items {
		if metav1.GetControllerOf(&machineSet) != nil && metav1.IsControlledBy(&machineSet, machineDeployment) {
			matchingMachineSets = append(matchingMachineSets, machineSet)
		}
	}
	return matchingMachineSets, nil
}

func hasMatchingLabels(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		glog.Warningf("unable to convert selector: %v", err)
		return false
	}
	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		glog.V(2).Infof("%v machineset has empty selector", machineSet.Name)
		return false
	}
	if !selector.Matches(labels.Set(machine.Labels)) {
		glog.V(4).Infof("%v machine has mismatch labels", machine.Name)
		return false
	}
	return true
}

func getInt32Ptr(i int32) *int32 {
	return &i
}
