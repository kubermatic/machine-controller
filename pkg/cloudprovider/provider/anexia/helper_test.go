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
	"testing"

	"github.com/gophercloud/gophercloud/testhelper"

	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
	providerconfigtypes "k8c.io/machine-controller/sdk/providerconfig"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type jsonObject = map[string]interface{}

type ProvisionVMTestCase struct {
	ReconcileContext reconcileContext
	AssertJSONBody   func(jsonBody jsonObject)
}

type ConfigTestCase struct {
	Config anxtypes.RawConfig
	Error  error
}

type ValidateCallTestCase struct {
	Spec          clusterv1alpha1.MachineSpec
	ExpectedError error
}

func getSpecsForValidationTest(t *testing.T, configCases []ConfigTestCase) []ValidateCallTestCase {
	var testCases []ValidateCallTestCase

	for _, configCase := range configCases {
		jsonConfig, err := json.Marshal(configCase.Config)
		testhelper.AssertNoErr(t, err)
		jsonProviderConfig, err := json.Marshal(providerconfigtypes.Config{
			CloudProviderSpec:   runtime.RawExtension{Raw: jsonConfig},
			OperatingSystemSpec: runtime.RawExtension{Raw: []byte("{}")},
		})
		testhelper.AssertNoErr(t, err)
		testCases = append(testCases, ValidateCallTestCase{
			Spec: clusterv1alpha1.MachineSpec{
				ProviderSpec: clusterv1alpha1.ProviderSpec{
					Value: &runtime.RawExtension{Raw: jsonProviderConfig},
				},
			},
			ExpectedError: configCase.Error,
		})
	}
	return testCases
}

func newConfigVarString(str string) providerconfigtypes.ConfigVarString {
	return providerconfigtypes.ConfigVarString{
		Value: str,
	}
}

// this generates a full config and allows hooking into it to e.g. remove a value.
func hookableConfig(hook func(*anxtypes.RawConfig)) anxtypes.RawConfig {
	config := anxtypes.RawConfig{
		CPUs: 1,

		Memory: 2,

		Disks: []anxtypes.RawDisk{
			{Size: 5, PerformanceType: newConfigVarString("ENT6")},
		},

		Networks: []anxtypes.RawNetwork{
			{VlanID: newConfigVarString("test-vlan"), PrefixIDs: []providerconfigtypes.ConfigVarString{newConfigVarString("test-prefix")}},
		},

		Token:      newConfigVarString("test-token"),
		LocationID: newConfigVarString("test-location"),
		TemplateID: newConfigVarString("test-template-id"),
	}

	if hook != nil {
		hook(&config)
	}

	return config
}

// this generates a full reconcileContext with some default values and allows hooking into it to e.g. remove/overwrite a value.
func hookableReconcileContext(locationID string, templateID string, hook func(*reconcileContext)) reconcileContext {
	context := reconcileContext{
		Machine: &clusterv1alpha1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "TestMachine"},
		},
		Status:   &anxtypes.ProviderStatus{},
		UserData: "",
		Config: resolvedConfig{
			LocationID: locationID,
			TemplateID: templateID,
			Disks: []resolvedDisk{
				{
					RawDisk: anxtypes.RawDisk{
						Size: 5,
					},
				},
			},
			Networks: []resolvedNetwork{
				{
					VlanID: "VLAN-ID",
					Prefixes: []string{
						"Prefix-ID",
					},
				},
			},
			RawConfig: anxtypes.RawConfig{
				CPUs:   5,
				Memory: 5,
			},
		},
		ProviderData: &cloudprovidertypes.ProviderData{
			Update: func(*clusterv1alpha1.Machine, ...cloudprovidertypes.MachineModifier) error {
				return nil
			},
		},
		ProviderConfig: &providerconfigtypes.Config{
			Network: &providerconfigtypes.NetworkConfig{
				DNS: providerconfigtypes.DNSConfig{
					Servers: []string{
						"1.1.1.1",
						"",
						"192.168.0.1",
						"192.168.0.2",
						"192.168.0.3",
					},
				},
			},
		},
	}

	if hook != nil {
		hook(&context)
	}

	return context
}
