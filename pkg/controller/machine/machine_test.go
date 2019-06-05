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

package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-test/deep"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

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
	machineinformer "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
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

func TestControllerShouldEvict(t *testing.T) {
	threeHoursAgo := metav1.NewTime(time.Now().Add(-3 * time.Hour))
	now := metav1.Now()

	tests := []struct {
		name               string
		machine            *clusterv1alpha1.Machine
		additionalMachines []runtime.Object
		existingNodes      []runtime.Object
		shouldEvict        bool
	}{
		{
			name:        "skip eviction due to eviction timeout",
			shouldEvict: false,
			existingNodes: []runtime.Object{&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-node",
				},
			}},
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &threeHoursAgo,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{Name: "existing-node"},
				},
			},
		},
		{
			name:        "skip eviction due to no nodeRef",
			shouldEvict: false,
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: nil,
				},
			},
		},
		{
			name:        "skip eviction due to already gone node",
			shouldEvict: false,
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{Name: "non-existing-node"},
				},
			},
		},
		{
			name: "Skip eviction due to no available target",
			existingNodes: []runtime.Object{&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-node",
				},
			}},
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{Name: "existing-node"},
				},
			},
		},
		{
			name:        "Eviction possible because of second node",
			shouldEvict: true,
			existingNodes: []runtime.Object{&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-node",
				}}, &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "eviction-destination",
				}},
			},
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{Name: "existing-node"},
				},
			},
		},
		{
			name:        "Eviction possible because of machine without noderef",
			shouldEvict: true,
			existingNodes: []runtime.Object{&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-node",
				}}, &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "eviction-destination",
				}},
			},
			machine: &clusterv1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: clusterv1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{Name: "existing-node"},
				},
			},
			additionalMachines: []runtime.Object{
				&clusterv1alpha1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "new-machine-without-a-node",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			kubeClient := fake.NewSimpleClientset(test.existingNodes...)
			if test.additionalMachines == nil {
				test.additionalMachines = []runtime.Object{}
			}
			machinefake := machinefake.NewSimpleClientset(append(test.additionalMachines, test.machine)...)
			informerFactory := informers.NewSharedInformerFactory(kubeClient, 5*time.Minute)
			machineInformerFactory := machineinformer.NewSharedInformerFactory(machinefake, 5*time.Minute)

			ctrl := &Controller{
				nodesLister:       informerFactory.Core().V1().Nodes().Lister(),
				machinesLister:    machineInformerFactory.Cluster().V1alpha1().Machines().Lister(),
				skipEvictionAfter: 2 * time.Hour,
			}

			informerFactory.Start(wait.NeverStop)
			informerFactory.WaitForCacheSync(wait.NeverStop)

			shouldEvict, err := ctrl.shouldEvict(test.machine)
			if err != nil {
				t.Fatal(err)
			}

			if shouldEvict != test.shouldEvict {
				t.Errorf("Expected shouldEvict to be %v but got %v instead", test.shouldEvict, shouldEvict)
			}
		})
	}
}
