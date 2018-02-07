package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machinev1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	machineReadyCheckPeriod  = 2 * time.Second
	machineReadyCheckTimeout = 5 * time.Minute
)

func main() {
	var manifestPath string
	var parameters string
	var kubeConfig string
	var nodeCount int
	var createOnly bool

	flag.StringVar(&kubeConfig, "kubeconfig", "", "a path to the kubeconfig.")
	flag.StringVar(&manifestPath, "input", "", "a path to the machine's manifest.")
	flag.StringVar(&parameters, "parameters", "", "a list of comma-delimited key value pairs i.e key=value,key1=value2.")
	flag.IntVar(&nodeCount, "nodeCount", 0, "the number of nodes that already exist in the cluster")
	flag.BoolVar(&createOnly, "createOnly", false, "if the tool should create only but not run deletion")
	flag.Parse()

	// input sanitizaiton
	if len(manifestPath) == 0 || len(parameters) == 0 || len(kubeConfig) == 0 {
		fmt.Println("please specify kubeconfig, input and parameters flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	keyValuePairs := strings.Split(parameters, ",")
	if len(keyValuePairs) == 0 {
		fmt.Println("incorrect value of parameters flag:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// init kube related stuff
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		printAndDie(fmt.Sprintf("error building kubeconfig: %v", err))
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		printAndDie(fmt.Sprintf("error building kubernetes clientset: %v", err))
	}
	machineClient, err := machineclientset.NewForConfig(cfg)
	if err != nil {
		printAndDie(fmt.Sprintf("error building example clientset: %v", err))
	}

	// prepare the manifest
	manifest, err := readAndModifyManifest(manifestPath, keyValuePairs)
	if err != nil {
		printAndDie(fmt.Sprintf("failed to prepare the manifest, due to: %v", err))
	}

	// act
	err = verify(manifest, kubeClient, machineClient, nodeCount, createOnly)
	if err != nil {
		printAndDie(fmt.Sprintf("failed to verify if a machine/node has been created/deleted, due to: \n%v", err))
	}
	msg := "all good, successfully verified that a machine/node has been created"
	if !createOnly {
		msg += " and then deleted"
	}
	fmt.Println(msg)
}

func verify(manifest string, kubeClient kubernetes.Interface, machineClient machineclientset.Interface, nodeCount int, createOnly bool) error {
	newMachine := &machinev1alpha1.Machine{}
	{
		manifestReader := strings.NewReader(manifest)
		manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
		err := manifestDecoder.Decode(newMachine)
		if err != nil {
			return err
		}
	}

	err := createAndAssure(newMachine, machineClient, kubeClient, nodeCount)
	if err != nil {
		return err
	}
	if createOnly {
		return nil
	}
	return deleteAndAssure(newMachine, machineClient, kubeClient)
}

func createAndAssure(machine *machinev1alpha1.Machine, machineClient machineclientset.Interface, kubeClient kubernetes.Interface, nodeCount int) error {
	// we expect to find no nodes within the cluster
	err := assureNodeCount(nodeCount, kubeClient)
	if err != nil {
		return fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	fmt.Printf("creating a new \"%s\" machine\n", machine.Name)
	_, err = machineClient.MachineV1alpha1().Machines().Create(machine)
	if err != nil {
		return err
	}
	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		err := assureNodeCount(nodeCount+1, kubeClient)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		status := getMachineStatusAsString(machine.Name, machineClient)
		return fmt.Errorf("falied to created the new machine, err = %v, machine Status = %v", err, status)
	}

	fmt.Printf("waiting for status = %s to come \n", v1.NodeReady)
	nodeName := machine.Spec.Name
	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		// assertion check - if true then something weird has happened
		// or someone else is playing with the cluster
		if len(nodes.Items) != nodeCount+1 {
			return false, fmt.Errorf("expected to get only %v node but got = %d", nodeCount+1, len(nodes.Items))
		}
		for _, node := range nodes.Items {
			for _, condition := range node.Status.Conditions {
				if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
					return true, nil
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

func deleteAndAssure(machine *machinev1alpha1.Machine, machineClient machineclientset.Interface, kubeClient kubernetes.Interface) error {
	fmt.Printf("deleting the machine \"%s\"\n", machine.Name)
	err := machineClient.MachineV1alpha1().Machines().Delete(machine.Name, nil)
	if err != nil {
		return fmt.Errorf("unable to remove machine %s, due to %v", machine.Name, err)
	}

	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		err := assureNodeCount(0, kubeClient)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("falied to delete the node, err = %v", err)
	}
	return nil
}

func assureNodeCount(expectedNodeCount int, kubeClient kubernetes.Interface) error {
	nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(nodes.Items) != expectedNodeCount {
		return fmt.Errorf("the current node count = %d is different than expected one = %d", len(nodes.Items), expectedNodeCount)
	}
	return nil
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

func getMachineStatusAsString(machineName string, machineClient machineclientset.Interface) string {
	statusMessage := ""

	machine, err := machineClient.MachineV1alpha1().Machines().Get(machineName, metav1.GetOptions{})
	if err == nil {
		if machine.Status.ErrorReason != nil {
			statusMessage = fmt.Sprintf("ErrorReason = %s", machine.Status.ErrorReason)
		}
		if machine.Status.ErrorMessage != nil {
			statusMessage = fmt.Sprintf("%s ErrorMessage", statusMessage, machine.Status.ErrorMessage)
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

func printAndDie(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}
