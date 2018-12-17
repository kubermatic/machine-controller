package admission

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestMachineDeploymentDefaulting(t *testing.T) {
	tests := []struct {
		name              string
		machineDeployment *clusterv1alpha1.MachineDeployment
		isValid           bool
	}{
		{
			name:              "Empty MachineDeployment validation should fail",
			machineDeployment: &clusterv1alpha1.MachineDeployment{},
			isValid:           false,
		},
		{
			name: "Minimal MachineDeployment validation should succeed",
			machineDeployment: &clusterv1alpha1.MachineDeployment{
				Spec: clusterv1alpha1.MachineDeploymentSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: clusterv1alpha1.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			isValid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machineDeploymentDefaultingFunction(test.machineDeployment)
			errs := validateMachineDeployment(*test.machineDeployment)
			if test.isValid != (len(errs) == 0) {
				t.Errorf("Expected machine to be valid: %t but got %d errors: %v", test.isValid, len(errs), errs)
			}
		})
	}
}
