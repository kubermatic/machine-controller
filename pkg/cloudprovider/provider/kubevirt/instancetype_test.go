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

package kubevirt

import (
	"context"
	"testing"

	kubevirtcorev1 "kubevirt.io/api/core/v1"
	kubevirtinstancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultVMInstanceType(t *testing.T) {
	const (
		namespace = "kubevirt-workloads"
		both      = "h200-worker"
		onlyNS    = "namespaced-only"
	)

	client := fakectrlruntimeclient.NewClientBuilder().WithObjects(
		&kubevirtinstancetypev1beta1.VirtualMachineInstancetype{ObjectMeta: metav1.ObjectMeta{Name: both, Namespace: namespace}},
		&kubevirtinstancetypev1beta1.VirtualMachineClusterInstancetype{ObjectMeta: metav1.ObjectMeta{Name: both}},
		&kubevirtinstancetypev1beta1.VirtualMachineInstancetype{ObjectMeta: metav1.ObjectMeta{Name: onlyNS, Namespace: namespace}},
	).Build()

	tests := []struct {
		name     string
		matcher  *kubevirtcorev1.InstancetypeMatcher
		wantKind string
		wantErr  bool
	}{
		{
			name:     "no kind prefers the namespaced instance type",
			matcher:  &kubevirtcorev1.InstancetypeMatcher{Name: both},
			wantKind: "VirtualMachineInstancetype",
		},
		{
			name:     "explicit cluster kind is honored when a namespaced one has the same name",
			matcher:  &kubevirtcorev1.InstancetypeMatcher{Name: both, Kind: "VirtualMachineClusterInstancetype"},
			wantKind: "VirtualMachineClusterInstancetype",
		},
		{
			name:     "explicit kind is matched case-insensitively",
			matcher:  &kubevirtcorev1.InstancetypeMatcher{Name: both, Kind: "virtualmachineclusterinstancetype"},
			wantKind: "VirtualMachineClusterInstancetype",
		},
		{
			name:     "explicit namespaced kind",
			matcher:  &kubevirtcorev1.InstancetypeMatcher{Name: both, Kind: "VirtualMachineInstancetype"},
			wantKind: "VirtualMachineInstancetype",
		},
		{
			name:    "explicit cluster kind does not fall back to a namespaced instance type",
			matcher: &kubevirtcorev1.InstancetypeMatcher{Name: onlyNS, Kind: "VirtualMachineClusterInstancetype"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultVMInstanceType(context.Background(), client, tt.matcher, namespace)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.matcher.Name || got.Kind != tt.wantKind {
				t.Errorf("got %s/%s, want %s/%s", got.Kind, got.Name, tt.wantKind, tt.matcher.Name)
			}
		})
	}
}
