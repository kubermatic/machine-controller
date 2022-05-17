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

package util

import (
	"context"
	"reflect"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoveFinalizerOnInstanceNotFound(t *testing.T) {
	if err := clusterv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add clusterv1alpha1 to scheme: %v", err)
	}

	var fakeClient = fakectrlruntimeclient.
		NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(&v1alpha1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test_machine",
				Finalizers: []string{
					"test_finalizer_1",
					"test_finalizer_2"},
			},
		}).
		Build()

	var testCases = []struct {
		name            string
		machine         *v1alpha1.Machine
		expectedMachine *v1alpha1.Machine
		providerData    *cloudprovidertypes.ProviderData
	}{
		{
			name: "Test remove machine finalizer",
			machine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					UID:  "123456",
					Name: "test_machine",
					Finalizers: []string{
						"test_finalizer_1",
						"test_finalizer_2"},
				},
			},
			expectedMachine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					UID:  "123456",
					Name: "test_machine",
					Finalizers: []string{
						"test_finalizer_2"},
				},
			},
			providerData: &cloudprovidertypes.ProviderData{
				Update: cloudprovidertypes.GetMachineUpdater(context.Background(), fakeClient),
				Client: fakeClient,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if removed, err := RemoveFinalizerOnInstanceNotFound("test_finalizer_1", tc.machine, tc.providerData); err != nil || !removed {
				t.Fatalf("failed removing finalizer: %v", err)
			}

			foundMachine := &v1alpha1.Machine{}
			if err := fakeClient.Get(
				context.Background(),
				types.NamespacedName{Name: "test_machine"},
				foundMachine); err != nil {
				t.Fatalf("failed to get machine: %v", err)
			}

			if !reflect.DeepEqual(foundMachine.Finalizers, tc.expectedMachine.Finalizers) {
				t.Fatalf("machine finalzers don't match, failed removing machine finalizers")
			}
		})
	}
}
