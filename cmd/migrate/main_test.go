package main

import (
	"testing"

	fakedownstreammachineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned/fake"
	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	fakeclusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func TestMigrateMachines(t *testing.T) {
	testcases := []struct {
		DownStreamMachine *machinev1alpha1downstream.Machine
		Node              *corev1.Node
		ErrExpected       bool
	}{
		{
			DownStreamMachine: &machinev1alpha1downstream.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-machine"},
				Spec:       machinev1alpha1downstream.MachineSpec{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}},
			},
			Node: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}},
		},
		// We currently fail if no corresponding node exists. Is this desired?
		{
			DownStreamMachine: &machinev1alpha1downstream.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-machine"},
				Spec:       machinev1alpha1downstream.MachineSpec{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}},
			},
			ErrExpected: true,
		},
	}

	for _, testCase := range testcases {
		downstreamFake := fakedownstreammachineclientset.NewSimpleClientset(testCase.DownStreamMachine)
		clusterV1alpha1Fake := fakeclusterv1alpha1clientset.NewSimpleClientset()
		var kubeFake *fakekube.Clientset
		if testCase.Node != nil {
			kubeFake = fakekube.NewSimpleClientset(testCase.Node)
		} else {
			kubeFake = fakekube.NewSimpleClientset()
		}

		err := migrateMachines(kubeFake, downstreamFake, clusterV1alpha1Fake)
		if err != nil {
			if testCase.ErrExpected {
				continue
			}
			t.Fatalf("migrateMachines failed: %v", err)
		}

		remainingDownstreamMachines, err := downstreamFake.MachineV1alpha1().Machines().List(metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list remaining downstreammachines: %v", err)
		}
		if len(remainingDownstreamMachines.Items) != 0 {
			t.Errorf("len(remainingDownstreamMachines) should be 0, was %v", len(remainingDownstreamMachines.Items))
		}

		existingUpstreamMachines, err := clusterV1alpha1Fake.ClusterV1alpha1().Machines("kube-system").List(metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list clusterv1alpha1 machines: %v", err)
		}
		if len(existingUpstreamMachines.Items) != 1 {
			t.Errorf("len(existingUpstreamMachines) should be 1, was %v", len(existingUpstreamMachines.Items))
		}

		node, err := kubeFake.CoreV1().Nodes().Get(existingUpstreamMachines.Items[0].Spec.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get node: %v", err)
		}
		if len(node.OwnerReferences) != 1 {
			t.Fatalf("Expected len(nodeOwnerRefernces) to be 1 but was %v", len(node.OwnerReferences))
		}

		if node.OwnerReferences[0].UID != existingUpstreamMachines.Items[0].UID {
			t.Fatalf("Epected node owner ref to match new machine owner ref!")
		}
	}

}
