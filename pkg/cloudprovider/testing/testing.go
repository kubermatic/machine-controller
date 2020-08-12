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

package testing

import (
	"testing"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProvProviderSpecGetter generates provider spec for testing purposes.
type ProviderSpecGetter func(t *testing.T) []byte

// Creator is used to generate test resources.
type Creator struct {
	Name               string
	Namespace          string
	ProviderSpecGetter ProviderSpecGetter
}

func (c Creator) CreateMachine(t *testing.T) *v1alpha1.Machine {
	return &v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
		Spec: v1alpha1.MachineSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.Name,
				Namespace: c.Namespace,
			},
			ProviderSpec: v1alpha1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: c.ProviderSpecGetter(t),
				},
			},
		},
	}
}
