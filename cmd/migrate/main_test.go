package main

import (
	"testing"

	fakedownstreammachineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned/fake"
	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	fakeclusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMigrateMachines(t *testing.T) {
	downstreamMachine := &machinev1alpha1downstream.Machine{ObjectMeta: metav1.ObjectMeta{Name: "test-machine"}}

	downstreamFake := fakedownstreammachineclientset.NewSimpleClientset(downstreamMachine)
	clusterV1alpha1Fake := fakeclusterv1alpha1clientset.NewSimpleClientset()

	if err := migrateMachines(downstreamFake, clusterV1alpha1Fake); err != nil {
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
}
