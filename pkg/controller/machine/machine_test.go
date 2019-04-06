package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	machinefake "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
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
	nodeList := []*corev1.Node{&node1, &node2, &node3}

	tests := []struct {
		name     string
		instance instance.Instance
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
		},
		{
			name:     "node not found - no suitable node",
			provider: "",
			resNode:  nil,
			exists:   false,
			err:      nil,
			instance: &fakeInstance{id: "99", addresses: []string{"192.168.1.99"}},
		},
		{
			name:     "node found by provider id",
			provider: "aws",
			resNode:  &node1,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "1", addresses: []string{""}},
		},
		{
			name:     "node found by internal ip",
			provider: "",
			resNode:  &node3,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "3", addresses: []string{"192.168.1.3"}},
		},
		{
			name:     "node found by external ip",
			provider: "",
			resNode:  &node3,
			exists:   true,
			err:      nil,
			instance: &fakeInstance{id: "3", addresses: []string{"172.16.1.3"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			for _, node := range nodeList {
				if err := nodeIndexer.Add(node); err != nil {
					t.Fatalf("failed to add node to nodeIndexer: %v", err)
				}
			}
			controller := Controller{nodesLister: corev1listers.NewNodeLister(nodeIndexer)}

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

func TestControllerDeletesMachinesOnJoinTimeout(t *testing.T) {
	tests := []struct {
		name              string
		creationTimestamp metav1.Time
		hasNode           bool
		ownerReferences   []metav1.OwnerReference
		hasOwner          bool
		getsDeleted       bool
		joinTimeoutConfig *time.Duration
	}{
		{
			name:              "machine with node does not get deleted",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
			hasNode:           true,
			getsDeleted:       false,
			joinTimeoutConfig: durationPtr(10 * time.Minute),
		},
		{
			name:              "machine without owner ref does not get deleted",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
			hasNode:           false,
			getsDeleted:       false,
			joinTimeoutConfig: durationPtr(10 * time.Minute),
		},
		{
			name:              "machine younger than joinClusterTimeout does not get deleted",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-9 * time.Minute)},
			hasNode:           false,
			ownerReferences:   []metav1.OwnerReference{{Name: "owner", Kind: "MachineSet"}},
			hasOwner:          true,
			getsDeleted:       false,
			joinTimeoutConfig: durationPtr(10 * time.Minute),
		},
		{
			name:              "machine older than joinClusterTimout gets deleted",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
			hasNode:           false,
			ownerReferences:   []metav1.OwnerReference{{Name: "owner", Kind: "MachineSet"}},
			getsDeleted:       true,
			joinTimeoutConfig: durationPtr(10 * time.Minute),
		},
		{
			name:              "machine older than joinClusterTimout doesnt get deletet when ownerReference.Kind != MachineSet",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
			hasNode:           false,
			ownerReferences:   []metav1.OwnerReference{{Name: "owner", Kind: "Cat"}},
			getsDeleted:       false,
			joinTimeoutConfig: durationPtr(10 * time.Minute),
		},
		{
			name:              "nil joinTimeoutConfig results in no deletions",
			creationTimestamp: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
			hasNode:           false,
			ownerReferences:   []metav1.OwnerReference{{Name: "owner", Kind: "MachineSet"}},
			getsDeleted:       false,
			joinTimeoutConfig: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machine := &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: test.creationTimestamp,
					OwnerReferences:   test.ownerReferences}}

			node := &corev1.Node{}
			instance := &fakeInstance{}
			if test.hasNode {
				literalNode := getTestNode("test-id", "")
				node = &literalNode
				instance.id = "test-id"
			}

			providerConfig := &providerconfig.Config{CloudProvider: providerconfig.CloudProviderFake}

			machineClient := machinefake.NewSimpleClientset(machine)

			nodeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			if err := nodeIndexer.Add(node); err != nil {
				t.Fatalf("failed to add node to nodeIndexer: %v", err)
			}

			controller := Controller{nodesLister: corev1listers.NewNodeLister(nodeIndexer),
				recorder:           &record.FakeRecorder{},
				machineClient:      machineClient,
				joinClusterTimeout: test.joinTimeoutConfig,
				workqueue:          workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 5*time.Minute), "Machines"),
			}

			if err := controller.ensureNodeOwnerRefAndConfigSource(instance, machine, providerConfig); err != nil {
				t.Fatalf("failed to call ensureNodeOwnerRefAndConfigSource: %v", err)
			}

			var wasDeleted bool
			for _, action := range machineClient.Actions() {
				if action.GetVerb() == "delete" {
					wasDeleted = true
					break
				}
			}

			if wasDeleted != test.getsDeleted {
				t.Errorf("Machine was deleted: %v, but expectedDeletion: %v", wasDeleted, test.getsDeleted)
			}
		})
	}

}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
