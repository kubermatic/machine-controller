package anexia

import (
	"encoding/json"
	"github.com/anexia-it/go-anxcloud/pkg/vsphere/search"
	"github.com/gophercloud/gophercloud/testhelper"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"testing"
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
