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

package anexia

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/anexia-it/go-anxcloud/pkg/vsphere/search"
	"github.com/gophercloud/gophercloud/testhelper"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"k8s.io/apimachinery/pkg/runtime"
)

type ConfigTestCase struct {
	Config anxtypes.RawConfig
	Error  error
}

type ValidateCallTestCase struct {
	Spec          v1alpha1.MachineSpec
	ExpectedError error
}

func getSpecsForValidationTest(t *testing.T, configCases []ConfigTestCase) []ValidateCallTestCase {

	var testCases []ValidateCallTestCase

	for _, configCase := range configCases {
		jsonConfig, err := json.Marshal(configCase.Config)
		testhelper.AssertNoErr(t, err)
		jsonProviderConfig, err := json.Marshal(types.Config{
			CloudProviderSpec:   runtime.RawExtension{Raw: jsonConfig},
			OperatingSystemSpec: runtime.RawExtension{Raw: []byte("{}")},
		})
		testhelper.AssertNoErr(t, err)
		testCases = append(testCases, ValidateCallTestCase{
			Spec: v1alpha1.MachineSpec{
				ProviderSpec: v1alpha1.ProviderSpec{
					Value: &runtime.RawExtension{Raw: jsonProviderConfig},
				},
			},
			ExpectedError: configCase.Error,
		})
	}
	return testCases
}

func createSearchHandler(t *testing.T, iterations int) http.HandlerFunc {
	counter := 0
	return func(writer http.ResponseWriter, request *http.Request) {
		test := request.URL.Query().Get("name")
		testhelper.AssertEquals(t, "%-TestMachine", test)
		testhelper.TestMethod(t, request, http.MethodGet)
		if iterations == counter {
			encoder := json.NewEncoder(writer)
			testhelper.AssertNoErr(t, encoder.Encode(map[string]interface{}{
				"data": []search.VM{
					{
						Name:       "543053-TestMachine",
						Identifier: TestIdentifier,
					},
				},
			}))
		}
		counter++
	}
}

func newConfigVarString(str string) types.ConfigVarString {
	return types.ConfigVarString{
		Value: str,
	}
}
