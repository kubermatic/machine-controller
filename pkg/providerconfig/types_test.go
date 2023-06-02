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

package providerconfig

import (
	"testing"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestDefaultOperatingSystemSpec(t *testing.T) {
	// this test validates that DefaultOperatingSystemSpec takes into accoult all listed operating systems in
	// AllOperatingSystems
	for _, osys := range providerconfigtypes.AllOperatingSystems {
		osys := osys
		t.Run(string(osys), func(t *testing.T) {
			operatingSystemSpec, err := DefaultOperatingSystemSpec(osys, "", runtime.RawExtension{}, true)

			if err != nil {
				t.Error("no error expected")
			}

			if operatingSystemSpec.Raw == nil {
				t.Errorf("expected not nil")
			}
		})
	}
}
