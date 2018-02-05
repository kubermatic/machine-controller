package main

import (
	"errors"
	"flag"
	"fmt"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machinev1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
	"time"
)

const (
	machineReadyCheckPeriod  = 2 * time.Second
	machineReadyCheckTimeout = 5 * time.Minute
)

func main() {
	var manifestPath string
	var parameters string
	var kubeConfig string

	flag.StringVar(&kubeConfig, "kubeconfig", "", "a path to the kubeconfig.")
	flag.StringVar(&manifestPath, "input", "", "a path to the machine's manifest.")
	flag.StringVar(&parameters, "parameters", "", "a list of comma-delimited key value pairs i.e key=value,key1=value2.")
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
		printAndDie(fmt.Sprintf("failed to prepare the manifest, due to = %v", err))
	}

	// act
	err = verify(manifest, kubeClient, machineClient)
	if err != nil {
		printAndDie(fmt.Sprintf("failed to verify if a machine/node has been created, reason: \n%v", err))
	}
	fmt.Println("all good, successfully verified that a machine/node has been created within the cluster")
}

func verify(manifest string, kubeClient kubernetes.Interface, machineClient machineclientset.Interface) error {
	newMachine := &machinev1alpha1.Machine{}
	{
		manifestReader := strings.NewReader(manifest)
		manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
		err := manifestDecoder.Decode(newMachine)
		if err != nil {
			return err
		}
	}

	// we expect to find no nodes within the cluster
	err := assureNodeCount(0, kubeClient)
	if err != nil {
		return fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	fmt.Printf("creating a new \"%s\" machine\n", newMachine.Name)
	_, err = machineClient.MachineV1alpha1().Machines().Create(newMachine)
	if err != nil {
		return err
	}
	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		err := assureNodeCount(1, kubeClient)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("falied to created the new machine, err = %v", err)
	}

	// TODO(lukasz): implement clean-up logic
	// TODO(lukasz): add dep
	return errors.New("not fully implemented")
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

func printAndDie(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}
