package controller

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-test/deep"
	machinefake "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned/fake"
	machinelistersv1alpha1 "github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type fakeInstance struct {
	name      string
	id        string
	addresses []string
	status    instance.Status
}

func (i *fakeInstance) Name() string {
	return i.name
}

func (i *fakeInstance) ID() string {
	return i.id
}

func (i *fakeInstance) Status() instance.Status {
	return i.status
}

func (i *fakeInstance) Addresses() []string {
	return i.addresses
}

func getTestNode(id, provider string) corev1.Node {
	providerID := ""
	if provider != "" {
		providerID = fmt.Sprintf("%s:///%s", provider, id)
	}
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("node%s", id),
		},
		Spec: corev1.NodeSpec{
			ProviderID: providerID,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: fmt.Sprintf("192.168.1.%s", id),
				},
				{
					Type:    corev1.NodeExternalIP,
					Address: fmt.Sprintf("172.16.1.%s", id),
				},
			},
		},
	}
}

func TestController_GetNode(t *testing.T) {
	node1 := getTestNode("1", "aws")
	node2 := getTestNode("2", "openstack")
	node3 := getTestNode("3", "")

	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			node1,
			node2,
			node3,
		},
	}

	tests := []struct {
		name     string
		instance instance.Instance
		objects  []runtime.Object
		resNode  *corev1.Node
		exists   bool
		err      error
		provider providerconfig.CloudProvider
	}{
		{
			name:     "node not found - no nodeList",
			provider: "",
			resNode:  nil,
			exists:   false,
			err:      nil,
			instance: &fakeInstance{id: "99", addresses: []string{"192.168.1.99"}},
			objects:  []runtime.Object{},
		},
		{
			name:     "node not found - no suitable node",
			provider: "",
			resNode:  nil,
			exists:   false,
			err:      nil,
			instance: &fakeInstance{id: "99", addresses: []string{"192.168.1.99"}},
			objects:  []runtime.Object{nodeList},
		},
		{
			name:     "node found by provider id",
			provider: "aws",
			resNode:  &node1,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "1", addresses: []string{""}},
			objects:  []runtime.Object{nodeList},
		},
		{
			name:     "node found by internal ip",
			provider: "",
			resNode:  &node3,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "3", addresses: []string{"192.168.1.3"}},
			objects:  []runtime.Object{nodeList},
		},
		{
			name:     "node found by external ip",
			provider: "",
			resNode:  &node3,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "3", addresses: []string{"172.16.1.3"}},
			objects:  []runtime.Object{nodeList},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := kubefake.NewSimpleClientset(test.objects...)
			kubeInformerFactory := kubeinformers.NewSharedInformerFactory(client, time.Second*30)
			nodeInformer := kubeInformerFactory.Core().V1().Nodes()
			go nodeInformer.Informer().Run(wait.NeverStop)
			cache.WaitForCacheSync(wait.NeverStop, nodeInformer.Informer().HasSynced)

			controller := Controller{nodesLister: nodeInformer.Lister()}

			node, exists, err := controller.getNode(test.instance, test.provider)
			if diff := deep.Equal(err, test.err); diff != nil {
				t.Errorf("expected to get %v instead got: %v", test.err, err)
			}
			if err != nil {
				return
			}

			if exists != test.exists {
				t.Errorf("expected to get %v instead got: %v", test.exists, exists)
			}
			if !exists {
				return
			}

			if diff := deep.Equal(node, test.resNode); diff != nil {
				t.Errorf("expected to get %v instead got: %v", test.resNode, node)
			}
		})
	}
}

func TestController_AddDeleteFinalizerOnlyIfValidationSucceeded(t *testing.T) {
	tests := []struct {
		name              string
		cloudProviderSpec string
		finalizerExpected bool
		err               string
		expectedActions   []string
	}{
		{
			name:              "Finalizer gets added on sucessfull validation",
			cloudProviderSpec: `{"passValidation": true}`,
			finalizerExpected: true,
			expectedActions:   []string{"update", "update", "update"},
		},
		{
			name:              "Finalizer does not get added on failed validation",
			cloudProviderSpec: `{"passValidation": false}`,
			err:               "invalid provider config: failing validation as requested",
			finalizerExpected: false,
			expectedActions:   []string{"update", "update", "update"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			providerConfig := fmt.Sprintf(`{"cloudProvider": "fake", "operatingSystem": "coreos",
		"cloudProviderSpec": %s}`, test.cloudProviderSpec)
			machine := &v1alpha1.Machine{}
			machine.Name = "testmachine"
			machine.Spec.ProviderConfig.Raw = []byte(providerConfig)

			fakeMachineClient := machinefake.NewSimpleClientset(runtime.Object(machine))
			machineIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			err := machineIndexer.Add(machine)
			if err != nil {
				t.Fatal(err)
			}
			machineLister := machinelistersv1alpha1.NewMachineLister(machineIndexer)
			nodeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			nodeLister := listerscorev1.NewNodeLister(nodeIndexer)

			controller := Controller{
				machineClient:   fakeMachineClient,
				machinesLister:  machineLister,
				metrics:         NewMachineControllerMetrics(),
				nodesLister:     nodeLister,
				recorder:        &record.FakeRecorder{},
				validationCache: map[string]bool{},
			}

			err = controller.syncHandler("testmachine")
			if err != nil && err.Error() != test.err {
				t.Fatalf("Expected test to have err '%s' but was '%v'", test.err, err)
			}
			if len(test.expectedActions) != len(fakeMachineClient.Actions()) {
				t.Fatalf("unexpected actions %#v", fakeMachineClient.Actions())
			}

			syncedMachine, err := fakeMachineClient.Machine().Machines().Get("testmachine", metav1.GetOptions{})
			if err != nil {
				t.Errorf("Failed to get machine: %v", err)
			}
			hasFinalizer := sets.NewString(syncedMachine.Finalizers...).Has(finalizerDeleteInstance)
			if hasFinalizer != test.finalizerExpected {
				t.Errorf("Finalizer expected: %v, but was:%v", test.finalizerExpected, hasFinalizer)
			}
		})
	}

}

func TestController_defaultContainerRuntime(t *testing.T) {
	tests := []struct {
		name    string
		machine *v1alpha1.Machine
		os      providerconfig.OperatingSystem
		err     error
		resCR   v1alpha1.ContainerRuntimeInfo
	}{
		{
			name:  "v1.9.2 ubuntu - get default container runtime",
			err:   nil,
			os:    providerconfig.OperatingSystemUbuntu,
			resCR: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: "17.03.2"},
			machine: &v1alpha1.Machine{
				Spec: v1alpha1.MachineSpec{
					Versions: v1alpha1.MachineVersionInfo{
						ContainerRuntime: v1alpha1.ContainerRuntimeInfo{Name: "", Version: ""},
						Kubelet:          "1.9.2",
					},
				},
			},
		},
		{
			name:  "v1.9.2 ubuntu - get default docker version",
			err:   nil,
			os:    providerconfig.OperatingSystemUbuntu,
			resCR: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: "17.03.2"},
			machine: &v1alpha1.Machine{
				Spec: v1alpha1.MachineSpec{
					Versions: v1alpha1.MachineVersionInfo{
						ContainerRuntime: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: ""},
						Kubelet:          "1.9.2",
					},
				},
			},
		},
		{
			name:  "v1.9.2 coreos - get default docker version",
			err:   nil,
			os:    providerconfig.OperatingSystemCoreos,
			resCR: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: "1.12.6"},
			machine: &v1alpha1.Machine{
				Spec: v1alpha1.MachineSpec{
					Versions: v1alpha1.MachineVersionInfo{
						ContainerRuntime: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: ""},
						Kubelet:          "1.9.2",
					},
				},
			},
		},
		{
			name:  "v1.9.2 ubuntu - get default cri-o version",
			err:   nil,
			os:    providerconfig.OperatingSystemUbuntu,
			resCR: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.CRIO, Version: "1.9"},
			machine: &v1alpha1.Machine{
				Spec: v1alpha1.MachineSpec{
					Versions: v1alpha1.MachineVersionInfo{
						ContainerRuntime: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.CRIO, Version: ""},
						Kubelet:          "1.9.2",
					},
				},
			},
		},
		{
			name:  "v1.8.5 ubuntu - no available cri-o version",
			err:   errors.New("no supported versions available for 'cri-o'"),
			os:    providerconfig.OperatingSystemUbuntu,
			resCR: v1alpha1.ContainerRuntimeInfo{},
			machine: &v1alpha1.Machine{
				Spec: v1alpha1.MachineSpec{
					Versions: v1alpha1.MachineVersionInfo{
						ContainerRuntime: v1alpha1.ContainerRuntimeInfo{Name: containerruntime.CRIO, Version: ""},
						Kubelet:          "v1.8.5",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := machinefake.NewSimpleClientset(test.machine)
			prov, err := userdata.ForOS(test.os)
			if err != nil {
				t.Fatal(err)
			}
			controller := Controller{machineClient: client}
			machine, err := controller.defaultContainerRuntime(test.machine, prov)
			if diff := deep.Equal(err, test.err); diff != nil {
				t.Errorf("expected to get '%v' instead got: '%v'", test.err, err)
			}
			if err != nil {
				return
			}

			cr := machine.Spec.Versions.ContainerRuntime
			if diff := deep.Equal(cr, test.resCR); diff != nil {
				t.Errorf("expected to get %s+%s instead got: %s+%s", test.resCR.Name, test.resCR.Version, cr.Name, cr.Version)
			}
		})
	}
}
