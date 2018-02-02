package main

import (
	"flag"
	"fmt"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	"io/ioutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
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
		printAndDie(fmt.Sprintf("failed to prepare the manifes, due to = %v", err))
	}

	// act
	err = verify(manifest, kubeClient, machineClient)
	if err != nil {
		printAndDie(fmt.Sprintf("failed to verify if a machine/node has been craeted, details = %v", err))
	}
	fmt.Println("all good, successfully verified that a machine/node has been created within the cluster")
}

func verify(manifest string, kubeClient kubernetes.Interface, machineClient machineclientset.Interface) error {
	return fmt.Errorf("not implemented")
}

func readAndModifyManifest(pathToManifest string, keyValuePairs []string) (string, error) {
	contentRaw, err := ioutil.ReadFile(pathToManifest)
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf("%s", contentRaw)

	for _, keyValuePair := range keyValuePairs {
		kv := strings.Split(keyValuePair, "=")
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
