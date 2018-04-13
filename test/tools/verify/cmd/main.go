package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"

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
	machineReadyCheckPeriod  = 5 * time.Second
	machineReadyCheckTimeout = 10 * time.Minute
	tempDir                  = "/tmp"
)

type arrayFlags string

func (i *arrayFlags) String() string {
	return string(*i)
}

func (i *arrayFlags) Set(value string) error {
	if value == "" {
		return nil
	}
	var direct arrayFlags
	if *i != arrayFlags("") {
		direct = arrayFlags(fmt.Sprintf("%s,%s", i, value))
	} else {
		direct = arrayFlags(value)
	}
	*i = direct

	return nil
}

func main() {
	var manifestPath string
	var parameters arrayFlags
	var kubeConfig string
	var createOnly bool

	defaultKubeconfigPath, err := getDefaultKubeconfigPath()
	if err != nil {
		glog.Fatalf("Error getting default path for kubeconfig: '%v'", err)
	}

	flag.StringVar(&kubeConfig, "kubeconfig", defaultKubeconfigPath, "a path to the kubeconfig.")
	flag.StringVar(&manifestPath, "input", "", "a path to the machine's manifest.")
	flag.Var(&parameters, "parameters", "a list of comma-delimited key value pairs i.e key=value,key1=value2. Can be passed multiple times")
	flag.BoolVar(&createOnly, "createOnly", false, "if the tool should create only but not run deletion")
	flag.Parse()

	// input sanitizaiton
	if len(manifestPath) == 0 || len(kubeConfig) == 0 {
		glog.Errorln("please specify kubeconfig and input flags!")
		flag.PrintDefaults()
		os.Exit(1)
	}
	keyValuePairs := strings.Split(parameters.String(), ",")
	if len(keyValuePairs) == 0 {
		glog.Errorln("incorrect value of parameters flag:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// init kube related stuff
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %v", err)
	}
	machineClient, err := machineclientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %v", err)
	}

	// prepare the manifest
	manifests, err := readAndModifyManifest(manifestPath, keyValuePairs)
	if err != nil {
		glog.Fatalf("Failed to prepare the manifest, due to: %v", err)
	}

	// Pragmatic solution for multiple manifests in one file
	manifestsList := strings.Split(manifests, "\n---\n")
	for _, manifest := range manifestsList {
		if manifest == "" {
			continue
		}
		if strings.Contains(manifest, "kind: Machine") {
			err = verify(manifest, kubeClient, machineClient, createOnly)
			if err != nil {
				glog.Fatalf("Failed to verify if a machine/node has been created/deleted, due to: \n%v", err)
				msg := "all good, successfully verified that a machine/node has been created"
				if !createOnly {
					msg += " and then deleted"
				}
				glog.Infoln(msg)
			}
		} else {
			// Be pragmatic
			glog.Infof("Trying to apply additional manifest...")
			err = kubectlApply(kubeConfig, manifest)
			if err != nil {
				glog.Fatalf("Error applying manifest: '%v'", err)
			}
			glog.Infof("Successfully applied additional manifest!")
		}
	}
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

func getDefaultKubeconfigPath() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(user.HomeDir, ".kube/config"), nil
}

func verify(manifest string, kubeClient kubernetes.Interface, machineClient machineclientset.Interface, createOnly bool) error {
	newMachine := &machinev1alpha1.Machine{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	err := manifestDecoder.Decode(newMachine)
	if err != nil {
		return err
	}

	err = createAndAssure(newMachine, machineClient, kubeClient)
	if err != nil {
		return err
	}
	if createOnly {
		return nil
	}
	return deleteAndAssure(newMachine, machineClient, kubeClient)
}

func createAndAssure(machine *machinev1alpha1.Machine, machineClient machineclientset.Interface, kubeClient kubernetes.Interface) error {
	// we expect that no node for machine exists in the cluster
	err := assureNodeForMachine(machine, kubeClient, false)
	if err != nil {
		return fmt.Errorf("unable to perform the verification, incorrect cluster state detected %v", err)
	}

	glog.Infof("creating a new \"%s\" machine\n", machine.Name)
	machine, err = machineClient.MachineV1alpha1().Machines().Create(machine)
	if err != nil {
		return err
	}
	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		err := assureNodeForMachine(machine, kubeClient, true)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		status := getMachineStatusAsString(machine.Name, machineClient)
		return fmt.Errorf("falied to created the new machine, err = %v, machine Status = %v", err, status)
	}

	glog.Infof("waiting for status = %s to come \n", v1.NodeReady)
	nodeName := machine.Spec.Name
	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
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

func deleteAndAssure(machine *machinev1alpha1.Machine, machineClient machineclientset.Interface, kubeClient kubernetes.Interface) error {
	glog.Infof("deleting the machine \"%s\"\n", machine.Name)
	err := machineClient.MachineV1alpha1().Machines().Delete(machine.Name, nil)
	if err != nil {
		return fmt.Errorf("unable to remove machine %s, due to %v", machine.Name, err)
	}

	err = wait.Poll(machineReadyCheckPeriod, machineReadyCheckTimeout, func() (bool, error) {
		err := assureNodeForMachine(machine, kubeClient, false)
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

// assureNodeForMachine according to shouldExists parameter check if a node for machine exists in the system or not
// this method examines OwnerReference of each node.
func assureNodeForMachine(machine *machinev1alpha1.Machine, kubeClient kubernetes.Interface, shouldExists bool) error {
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

func isNodeForMachine(node *v1.Node, machine *machinev1alpha1.Machine) bool {
	ownerRef := metav1.GetControllerOf(node)
	if ownerRef == nil {
		return false
	}
	return ownerRef.Name == machine.Name && ownerRef.UID == machine.UID
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
