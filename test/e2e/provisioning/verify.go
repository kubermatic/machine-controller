/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioning

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	evictiontypes "github.com/kubermatic/machine-controller/pkg/node/eviction/types"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	machineReadyCheckPeriod = 15 * time.Second
)

func verifyCreateMachineFails(kubeConfig, manifestPath string, parameters []string, _ time.Duration) error {
	client, machine, err := prepareMachine(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}
	if err := client.Create(context.Background(), machine); err != nil {
		return nil
	}
	return fmt.Errorf("expected create of Machine %s to fail but succeeded", machine.Name)
}

func verifyCreateAndDelete(kubeConfig, manifestPath string, parameters []string, timeout time.Duration) error {
	client, machineDeployment, err := prepareMachineDeployment(kubeConfig, manifestPath, parameters)
	if err != nil {
		return err
	}

	machineDeployment, err = createAndAssure(machineDeployment, client, timeout)
	if err != nil {
		return fmt.Errorf("failed to verify creation of node for MachineDeployment: %w", err)
	}

	if err := deleteAndAssure(machineDeployment, client, timeout); err != nil {
		return fmt.Errorf("Failed to verify if a machine/node has been created/deleted, due to: \n%w", err)
	}

	klog.Infof("Successfully finished test for MachineDeployment %s", machineDeployment.Name)
	return nil
}

func prepareMachineDeployment(kubeConfig, manifestPath string, parameters []string) (ctrlruntimeclient.Client, *clusterv1alpha1.MachineDeployment, error) {
	client, manifest, err := prepare(kubeConfig, manifestPath, parameters)
	if err != nil {
		return nil, nil, err
	}
	newMachineDeployment := &clusterv1alpha1.MachineDeployment{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	err = manifestDecoder.Decode(newMachineDeployment)
	if err != nil {
		return nil, nil, err
	}
	// Enforce the kube-system namespace, otherwise cleanup won't work
	newMachineDeployment.Namespace = metav1.NamespaceSystem
	// Dont evict during testing
	newMachineDeployment.Spec.Template.Spec.Annotations = map[string]string{evictiontypes.SkipEvictionAnnotationKey: "true"}

	return client, newMachineDeployment, nil
}

func prepareMachine(kubeConfig, manifestPath string, parameters []string) (ctrlruntimeclient.Client, *clusterv1alpha1.Machine, error) {
	client, manifest, err := prepare(kubeConfig, manifestPath, parameters)
	if err != nil {
		return nil, nil, err
	}
	newMachine := &clusterv1alpha1.Machine{}
	manifestReader := strings.NewReader(manifest)
	manifestDecoder := yaml.NewYAMLToJSONDecoder(manifestReader)
	err = manifestDecoder.Decode(newMachine)
	if err != nil {
		return nil, nil, err
	}
	// Enforce the kube-system namespace, otherwise cleanup won't work
	newMachine.Namespace = metav1.NamespaceSystem
	// Dont evict during testing
	newMachine.Spec.Annotations = map[string]string{evictiontypes.SkipEvictionAnnotationKey: "true"}

	return client, newMachine, nil
}

func prepare(kubeConfig, manifestPath string, parameters []string) (ctrlruntimeclient.Client, string, error) {
	if len(manifestPath) == 0 || len(kubeConfig) == 0 {
		return nil, "", fmt.Errorf("kubeconfig and manifest path must be defined")
	}

	// init kube related stuff
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, "", fmt.Errorf("Error building kubeconfig: %w", err)
	}
	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Client: %w", err)
	}

	// prepare the manifest
	manifest, err := readAndModifyManifest(manifestPath, parameters)
	if err != nil {
		return nil, "", fmt.Errorf("failed to prepare the manifest, due to: %w", err)
	}

	return client, manifest, nil
}

func createAndAssure(machineDeployment *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Client, timeout time.Duration) (*clusterv1alpha1.MachineDeployment, error) {
	// we expect that no node for machine exists in the cluster
	err := assureNodeForMachineDeployment(machineDeployment, client, false)
	if err != nil {
		return nil, fmt.Errorf("unable to perform the verification, incorrect cluster state detected %w", err)
	}

	klog.Infof("Creating a new %q MachineDeployment", machineDeployment.Name)

	// Some cloud provider API's are slow (e.g. hetzner), and it can happen that our webhook
	// needs longer to validate a MachineDeployment than the kube-apiserver is willing to wait.
	// In real world scenarios this is not that critical, but for tests we need to pay closer
	// attention and retry the creation a few times.
	err = wait.PollImmediate(3*time.Second, 180*time.Second, func() (bool, error) {
		err := client.Create(context.Background(), machineDeployment)
		if err != nil {
			klog.Warningf("Creation of %q failed, retrying: %v", machineDeployment.Name, err)
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MachineDeployment %q: %w", machineDeployment.Name, err)
	}

	klog.Infof("MachineDeployment %q created", machineDeployment.Name)

	var pollErr error
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		pollErr = assureNodeForMachineDeployment(machineDeployment, client, true)
		if pollErr == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed waiting for MachineDeployment %s to get a node: %w (%v)", machineDeployment.Name, err, pollErr)
	}
	klog.Infof("Found a node for MachineDeployment %s", machineDeployment.Name)

	klog.Infof("Waiting for node of MachineDeployment %s to become ready", machineDeployment.Name)
	err = wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		machines, pollErr := getMatchingMachines(machineDeployment, client)
		if pollErr != nil || len(machines) < 1 {
			return false, nil
		}
		for _, machine := range machines {
			hasReadyNode, pollErr := hasMachineReadyNode(&machine, client)
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
		return nil, fmt.Errorf("failed waiting for MachineDeployment %s to get a node in ready state: %w", machineDeployment.Name, err)
	}
	return machineDeployment, nil
}

func hasMachineReadyNode(machine *clusterv1alpha1.Machine, client ctrlruntimeclient.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	if err := client.List(context.Background(), nodes); err != nil {
		return false, fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, node := range nodes.Items {
		if isNodeForMachine(&node, machine) {
			foundConditions := make(map[corev1.NodeConditionType]corev1.ConditionStatus)

			for _, condition := range node.Status.Conditions {
				foundConditions[condition.Type] = condition.Status
				// TODO: re-enable this once you figure out how to properly run a CNI
				//	if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				//    return true, nil
				// }
			}

			// ensure that kubelet self-reported resource health
			return foundConditions[corev1.NodeMemoryPressure] == corev1.ConditionFalse &&
				foundConditions[corev1.NodeDiskPressure] == corev1.ConditionFalse &&
				foundConditions[corev1.NodePIDPressure] == corev1.ConditionFalse, nil
		}
	}
	return false, nil
}

func deleteAndAssure(machineDeployment *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Client, timeout time.Duration) error {
	klog.Infof("Starting to clean up MachineDeployment %s", machineDeployment.Name)

	// We first scale down to 0, because once the machineSets are deleted we can not
	// match machines anymore and we do want to verify not only the node is gone but also
	// the instance at the cloud provider
	if err := updateMachineDeployment(machineDeployment, client, func(md *clusterv1alpha1.MachineDeployment) {
		md.Spec.Replicas = getInt32Ptr(0)
	}); err != nil {
		return fmt.Errorf("failed to update replicas of MachineDeployment %s: %w", machineDeployment.Name, err)
	}

	// Ensure machines are gone
	if err := wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		ownedMachines, err := getMatchingMachines(machineDeployment, client)
		if err != nil {
			return false, err
		}
		if len(ownedMachines) != 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for machines of MachineDeployment %s to be deleted: %w", machineDeployment.Name, err)
	}

	klog.V(2).Infof("Deleting MachineDeployment %s", machineDeployment.Name)
	if err := client.Delete(context.Background(), machineDeployment); err != nil {
		return fmt.Errorf("unable to remove MachineDeployment %s, due to %w", machineDeployment.Name, err)
	}
	return wait.Poll(machineReadyCheckPeriod, timeout, func() (bool, error) {
		err := client.Get(context.Background(), types.NamespacedName{Namespace: machineDeployment.Namespace, Name: machineDeployment.Name}, &clusterv1alpha1.MachineDeployment{})
		if kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

// assureNodeForMachineDeployment according to shouldExists parameter check if a node for machine exists in the system or not
// this method examines OwnerReference of each node.
func assureNodeForMachineDeployment(machineDeployment *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Client, shouldExist bool) error {
	machines, err := getMatchingMachines(machineDeployment, client)
	if err != nil {
		return fmt.Errorf("failed to list Machines: %w", err)
	}

	if shouldExist {
		if len(machines) == 0 {
			return fmt.Errorf("expected to find a node for MachineDeployment %q but it has no Machine", machineDeployment.Name)
		}

		for _, machine := range machines {
			// Azure doesn't seem to easily expose the private IP address, there is only a PublicIPAddressClient in the sdk
			providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
			if err != nil {
				return fmt.Errorf("failed to get provider config: %w", err)
			}
			if providerConfig.CloudProvider == providerconfigtypes.CloudProviderAzure {
				continue
			}

			if len(machine.Status.Addresses) == 0 {
				return fmt.Errorf("expected to find a node for MachineDeployment %q but Machine %q has no address yet, indicating instance creation at the provider failed", machineDeployment.Name, machine.Name)
			}
		}
	}

	nodes := &corev1.NodeList{}
	if err := client.List(context.Background(), nodes); err != nil {
		return fmt.Errorf("failed to list Nodes: %w", err)
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

	if shouldExist != nodeForMachineExists {
		return fmt.Errorf("expected node to exists=%v but could find node=%v", shouldExist, nodeForMachineExists)
	}
	return nil
}

func isNodeForMachine(node *corev1.Node, machine *clusterv1alpha1.Machine) bool {
	// This gets called before the Objects are persisted in the API
	// which means UI will be emppy for machine
	if string(machine.UID) == "" {
		return false
	}
	return node.Labels[machinecontroller.NodeOwnerLabelName] == string(machine.UID)
}

func readAndModifyManifest(pathToManifest string, keyValuePairs []string) (string, error) {
	contentRaw, err := os.ReadFile(pathToManifest)
	if err != nil {
		return "", err
	}
	content := string(contentRaw)

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

// getMatchingMachines returns all machines that are owned by the passed machineDeployment.
func getMatchingMachines(machineDeployment *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Client) ([]clusterv1alpha1.Machine, error) {
	matchingMachineSets, err := getMatchingMachineSets(machineDeployment, client)
	if err != nil {
		return nil, err
	}
	klog.V(2).Infof("Found %v matching MachineSets for %s", len(matchingMachineSets), machineDeployment.Name)
	var matchingMachines []clusterv1alpha1.Machine
	for _, machineSet := range matchingMachineSets {
		machinesForMachineSet, err := getMatchingMachinesForMachineset(&machineSet, client)
		if err != nil {
			return nil, fmt.Errorf("failed to get matching Machines for MachineSet %s: %w", machineSet.Name, err)
		}
		matchingMachines = append(matchingMachines, machinesForMachineSet...)
	}
	klog.V(2).Infof("Found %v matching Machines for MachineDeployment %s", len(matchingMachines), machineDeployment.Name)
	return matchingMachines, nil
}

func getMatchingMachinesForMachineset(machineSet *clusterv1alpha1.MachineSet, client ctrlruntimeclient.Client) ([]clusterv1alpha1.Machine, error) {
	allMachines := &clusterv1alpha1.MachineList{}
	if err := client.List(context.Background(), allMachines, &ctrlruntimeclient.ListOptions{Namespace: machineSet.Namespace}); err != nil {
		return nil, fmt.Errorf("failed to list Machines: %w", err)
	}
	var matchingMachines []clusterv1alpha1.Machine
	for _, machine := range allMachines.Items {
		if metav1.GetControllerOf(&machine) != nil && metav1.IsControlledBy(&machine, machineSet) {
			matchingMachines = append(matchingMachines, machine)
		}
	}
	return matchingMachines, nil
}

// getMatchingMachineSets returns all machineSets that are owned by the passed machineDeployment.
func getMatchingMachineSets(machineDeployment *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Reader) ([]clusterv1alpha1.MachineSet, error) {
	// Ensure we actually have an object from the KubeAPI and not just the result of the yaml parsing, as the latter
	// can not be the owner of anything due to missing UID.
	if machineDeployment.ResourceVersion == "" {
		nn := types.NamespacedName{Namespace: machineDeployment.Namespace, Name: machineDeployment.Name}
		if err := client.Get(context.Background(), nn, machineDeployment); err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get MachineDeployment %s: %w", nn.Name, err)
			}
			return nil, nil
		}
	}
	allMachineSets := &clusterv1alpha1.MachineSetList{}
	if err := client.List(context.Background(), allMachineSets, &ctrlruntimeclient.ListOptions{Namespace: machineDeployment.Namespace}); err != nil {
		return nil, fmt.Errorf("failed to list MachineSets: %w", err)
	}
	var matchingMachineSets []clusterv1alpha1.MachineSet
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

func updateMachineDeployment(md *clusterv1alpha1.MachineDeployment, client ctrlruntimeclient.Client, modify func(*clusterv1alpha1.MachineDeployment)) error {
	// Store Namespace and Name here because after an error md will be nil
	name := md.Name
	namespace := md.Namespace

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		md := &clusterv1alpha1.MachineDeployment{}
		if err := client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, md); err != nil {
			return err
		}
		modify(md)
		return client.Update(context.Background(), md)
	})
}
