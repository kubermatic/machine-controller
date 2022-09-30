/*
Copyright 2022 The Machine Controller Authors.

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

package kubevirt

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTopologySpreadConstraint(t *testing.T) {
	tests := []struct {
		desc     string
		config   Config
		expected []corev1.TopologySpreadConstraint
	}{
		{
			desc:   "default topology constraint",
			config: Config{TopologySpreadConstraints: nil},
			expected: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: topologyKeyHostname, WhenUnsatisfiable: corev1.ScheduleAnyway, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"md": "test-md"}}},
			},
		},
		{
			desc:   "custom topology constraint",
			config: Config{TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: "test-topology-key", WhenUnsatisfiable: corev1.DoNotSchedule}}},
			expected: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "test-topology-key", WhenUnsatisfiable: corev1.DoNotSchedule, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"md": "test-md"}}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := getTopologySpreadConstraints(&test.config, map[string]string{"md": "test-md"})
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("expected ToplogySpreadConstraint: %v, got: %v", test.expected, result)
			}
		})
	}
}
