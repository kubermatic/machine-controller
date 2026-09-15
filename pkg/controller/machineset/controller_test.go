/*
Copyright 2026 The Machine Controller Authors.

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

package machineset

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	machinecontroller "k8c.io/machine-controller/pkg/controller/machine"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
)

func TestCreateMachineCopiesEvictionTimeoutOverride(t *testing.T) {
	tests := []struct {
		name           string
		machineSet     *clusterv1alpha1.MachineSet
		expectOverride bool
	}{
		{
			name: "MachineSet object annotation lands on created Machines",
			machineSet: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
						"unrelated.example/some-other-annotation":     "should-not-leak",
					},
				},
				Spec: clusterv1alpha1.MachineSetSpec{
					Template: clusterv1alpha1.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"machine": "worker"},
						},
					},
				},
			},
			expectOverride: true,
		},
		{
			name: "no override annotation, nothing copied",
			machineSet: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						"unrelated.example/some-other-annotation": "should-not-leak",
					},
				},
			},
			expectOverride: false,
		},
		{
			name: "nil annotations, nothing copied",
			machineSet: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
				},
			},
			expectOverride: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &ReconcileMachineSet{}
			created := r.createMachine(test.machineSet)

			value, exists := created.Annotations[machinecontroller.AnnotationSkipEvictionAfter]
			if test.expectOverride {
				if !exists {
					t.Fatalf("Expected created Machine to carry the %s annotation", machinecontroller.AnnotationSkipEvictionAfter)
				}
				if value != "45m" {
					t.Errorf("Expected override value %q to be copied verbatim, got %q", "45m", value)
				}
			} else if exists {
				t.Errorf("Expected created Machine not to carry the %s annotation, got %q", machinecontroller.AnnotationSkipEvictionAfter, value)
			}

			if v, leak := created.Annotations["unrelated.example/some-other-annotation"]; leak && v == "should-not-leak" && !test.expectOverride {
				// Only the override annotation is copied from the MachineSet object;
				// the unrelated annotation must never land on the Machine via this path.
				t.Errorf("Unrelated MachineSet object annotation leaked onto the created Machine")
			}
		})
	}
}
