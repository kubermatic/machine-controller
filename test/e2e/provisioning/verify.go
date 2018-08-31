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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
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
	clusterClient, err := clusterv1alpha1clientset.NewForConfig(cfg)
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
		if strings.Contains(manifest, "kind: Machine") {
			newMachine := &clusterv1alpha1.Machine{}
			manifestReader := strings.NewReader(manifest)
			manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
			err = manifestDecoder.Decode(newMachine)
			if err != nil {
				return err
			}

			err = createAndAssure(newMachine, clusterClient, kubeClient, timeout)
			if err != nil {
				return err
			}

			err = deleteAndAssure(newMachine, clusterClient, kubeClient, timeout)
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

func createAndAssure(machine *clusterv1alpha1.Machine,
	clusterClient clusterv1alpha1clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) error {
	// we expect that no node for machine exists in the cluster
	err := assureNodeForMachine(machine, kubeClient, false)
	if err != nil {
		return fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	glog.Infof("creating a new \"%s\" machine\n", machine.Name)
	machine, err = clusterClient.ClusterV1alpha1().Machines(machine.Namespace).Create(machine)
	if err != nil {
		return err
	}
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		pollErr := assureNodeForMachine(machine, kubeClient, true)
		if pollErr == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		status := getMachineStatusAsString(machine, clusterClient)
		return fmt.Errorf("falied to created the new machine, err = %v, machine Status = %v", err, status)
	}

	glog.Infof("waiting for status = %s to come \n", v1.NodeReady)
	nodeName := machine.Spec.Name
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		nodes, pollErr := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if pollErr != nil {
			return false, nil
		}

		for _, node := range nodes.Items {
			if isNodeForMachine(&node, machine) {
				for _, condition := range node.Status.Conditions {
					if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
						return true, nil
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		status := getNodeStatusAsString(nodeName, kubeClient)
		return fmt.Errorf("falied to created the new machine, err = %v, node Status %v", err, status)
	}
	return nil
}

func deleteAndAssure(machine *clusterv1alpha1.Machine,
	clusterClient clusterv1alpha1clientset.Interface, kubeClient kubernetes.Interface, timeout time.Duration) error {
	glog.Infof("deleting the machine \"%s\"\n", machine.Name)
	err := clusterClient.ClusterV1alpha1().Machines(machine.Namespace).Delete(machine.Name, nil)
	if err != nil {
		return fmt.Errorf("unable to remove machine %s, due to %v", machine.Name, err)
	}

	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		// errNodeStillExists is nil if the node is absent
		errNodeStillExists := assureNodeForMachine(machine, kubeClient, false)
		if errNodeStillExists != nil {
			return false, nil
		}
		_, errGetMachine := clusterClient.ClusterV1alpha1().Machines(machine.Namespace).Get(machine.Name, metav1.GetOptions{})
		if errGetMachine != nil && kerrors.IsNotFound(errGetMachine) {
			return true, nil
		}
		return false, errGetMachine
	})
	if err != nil {
		return fmt.Errorf("falied to delete the node, err = %v", err)
	}
	return nil
}

// assureNodeForMachine according to shouldExists parameter check if a node for machine exists in the system or not
// this method examines OwnerReference of each node.
func assureNodeForMachine(machine *clusterv1alpha1.Machine, kubeClient kubernetes.Interface, shouldExists bool) error {
	nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	nodeForMachineExists := false
	for _, node := range nodes.Items {
		if isNodeForMachine(&node, machine) {
			nodeForMachineExists = true
			break
		}
	}

	if shouldExists != nodeForMachineExists {
		return fmt.Errorf("expeced a node in the system to exists=%v but have found a node in the current cluster=%v", shouldExists, nodeForMachineExists)
	}
	return nil
}

func isNodeForMachine(node *v1.Node, machine *clusterv1alpha1.Machine) bool {
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

func getMachineStatusAsString(machine *clusterv1alpha1.Machine, machineClient clusterv1alpha1clientset.Interface) string {
	statusMessage := ""

	machine, err := machineClient.ClusterV1alpha1().Machines(machine.Namespace).Get(machine.Name, metav1.GetOptions{})
	if err == nil {
		if machine.Status.ErrorReason != nil {
			statusMessage = fmt.Sprintf("ErrorReason = %s", *machine.Status.ErrorReason)
		}
		if machine.Status.ErrorMessage != nil {
			statusMessage = fmt.Sprintf("%s ErrorMessage: '%s'", statusMessage, *machine.Status.ErrorMessage)
		}
	}

	return strings.Trim(statusMessage, " ")
}

func getNodeStatusAsString(nodeName string, kubeClient kubernetes.Interface) string {
	statusMessage := ""

	node, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err == nil {
		for _, condition := range node.Status.Conditions {
			statusMessage = fmt.Sprintf("%s %s = %s", statusMessage, condition.Type, condition.Reason)
		}
	}

	return strings.Trim(statusMessage, " ")
}
