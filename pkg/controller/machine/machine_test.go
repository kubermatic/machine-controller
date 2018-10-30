package controller

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/clusterinfo"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterfake "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	clusterinformers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
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
			providerConfig := fmt.Sprintf(`{"cloudProvider": "fake", "operatingSystem": "coreos",
		"cloudProviderSpec": %s}`, test.cloudProviderSpec)
			machine := &v1alpha1.Machine{}
			machine.Name = "testmachine"
			machine.Namespace = "kube-system"
			machine.Spec.ProviderConfig.Value = &runtime.RawExtension{Raw: []byte(providerConfig)}

			controller, fakeMachineClient := createTestMachineController(t, machine)

			if err := controller.syncHandler("kube-system/testmachine"); err != nil && err.Error() != test.err {
				t.Fatalf("Expected test to have err '%s' but was '%v'", test.err, err)
			}

			syncedMachine, err := fakeMachineClient.Cluster().Machines("kube-system").Get("testmachine", metav1.GetOptions{})
			if err != nil {
				t.Errorf("Failed to get machine: %v", err)
			}
			hasFinalizer := sets.NewString(syncedMachine.Finalizers...).Has(FinalizerDeleteInstance)
			if hasFinalizer != test.finalizerExpected {
				t.Errorf("Finalizer expected: %v, but was:%v", test.finalizerExpected, hasFinalizer)
			}
		})
	}

}

func createTestMachineController(t *testing.T, objects ...runtime.Object) (*Controller, *clusterfake.Clientset) {
	kubeClient := kubefake.NewSimpleClientset()
	clusterClient := clusterfake.NewSimpleClientset(objects...)

	kubeInformersFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Millisecond*50)
	kubeSystemInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Millisecond*50)
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, time.Millisecond*50)

	ctrl := NewMachineController(
		kubeClient,
		clusterClient,
		kubeInformersFactory.Core().V1().Nodes().Informer(),
		kubeInformersFactory.Core().V1().Nodes().Lister(),
		clusterInformerFactory.Cluster().V1alpha1().Machines().Informer(),
		clusterInformerFactory.Cluster().V1alpha1().Machines().Lister(),
		kubeSystemInformerFactory.Core().V1().Secrets().Lister(),
		[]net.IP{},
		NewMachineControllerMetrics(),
		nil,
		&clusterinfo.KubeconfigProvider{},
		"")

	clusterInformerFactory.Start(wait.NeverStop)
	kubeInformersFactory.Start(wait.NeverStop)

	clusterInformerFactory.WaitForCacheSync(wait.NeverStop)
	kubeInformersFactory.WaitForCacheSync(wait.NeverStop)

	return ctrl, clusterClient
}
