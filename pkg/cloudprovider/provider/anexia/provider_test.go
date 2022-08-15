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
	"errors"
	"net/http"
	"testing"
	"time"

	anxclient "github.com/anexia-it/go-anxcloud/pkg/client"
	"github.com/anexia-it/go-anxcloud/pkg/ipam/address"
	"github.com/anexia-it/go-anxcloud/pkg/vsphere/provisioning/progress"
	"github.com/anexia-it/go-anxcloud/pkg/vsphere/provisioning/vm"
	"github.com/gophercloud/gophercloud/testhelper"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const TestIdentifier = "TestIdent"

func TestAnexiaProvider(t *testing.T) {
	testhelper.SetupHTTP()
	client, server := anxclient.NewTestClient(nil, testhelper.Mux)
	t.Cleanup(func() {
		testhelper.TeardownHTTP()
		server.Close()
	})

	t.Run("Test waiting for VM", func(t *testing.T) {
		t.Parallel()

		waitUntilVMIsFound := 2
		testhelper.Mux.HandleFunc("/api/vsphere/v1/search/by_name.json", createSearchHandler(t, waitUntilVMIsFound))

		providerStatus := anxtypes.ProviderStatus{}
		ctx := createReconcileContext(reconcileContext{
			Machine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "TestMachine"},
			},
			Status:   &providerStatus,
			UserData: "",
			Config:   resolvedConfig{},

			ProviderData: &cloudprovidertypes.ProviderData{
				Update: func(m *clusterv1alpha1.Machine, mod ...cloudprovidertypes.MachineModifier) error {
					return nil
				},
			},
		})

		err := waitForVM(ctx, client)
		if err != nil {
			t.Fatal("No error was expected", err)

		}

		if providerStatus.InstanceID != TestIdentifier {
			t.Errorf("Excpected InstanceID to be set")
		}
	})

	t.Run("Test provision VM", func(t *testing.T) {
		t.Parallel()
		testhelper.Mux.HandleFunc("/api/ipam/v1/address/reserve/ip/count.json", func(writer http.ResponseWriter, request *http.Request) {
			err := json.NewEncoder(writer).Encode(address.ReserveRandomSummary{
				Data: []address.ReservedIP{
					{
						ID:      "IP-ID",
						Address: "8.8.8.8",
					},
				},
			})
			testhelper.AssertNoErr(t, err)
		})

		testhelper.Mux.HandleFunc("/api/vsphere/v1/provisioning/vm.json/LOCATION-ID/templates/TEMPLATE-ID", func(writer http.ResponseWriter, request *http.Request) {
			testhelper.TestMethod(t, request, http.MethodPost)
			type jsonObject = map[string]interface{}
			expectedJSON := map[string]interface{}{
				"cpu_performance_type": "performance",
				"hostname":             "TestMachine",
				"memory_mb":            json.Number("5"),
				"network": []jsonObject{
					{
						"vlan":     "VLAN-ID",
						"nic_type": "vmxnet3",
						"ips":      []interface{}{"8.8.8.8"},
					},
				},
			}
			var jsonBody jsonObject
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			testhelper.AssertNoErr(t, decoder.Decode(&jsonBody))
			testhelper.AssertEquals(t, expectedJSON["cpu_performance_type"], jsonBody["cpu_performance_type"])
			testhelper.AssertEquals(t, expectedJSON["hostname"], jsonBody["hostname"])
			testhelper.AssertEquals(t, expectedJSON["memory_mb"], jsonBody["memory_mb"])
			testhelper.AssertEquals(t, expectedJSON["count"], jsonBody["count"])

			expectedNetwork := expectedJSON["network"].([]jsonObject)[0]
			bodyNetwork := jsonBody["network"].([]interface{})[0].(jsonObject)
			testhelper.AssertEquals(t, expectedNetwork["vlan"], bodyNetwork["vlan"])
			testhelper.AssertEquals(t, expectedNetwork["nic_type"], bodyNetwork["nic_type"])
			testhelper.AssertEquals(t, expectedNetwork["ips"].([]interface{})[0], bodyNetwork["ips"].([]interface{})[0])

			err := json.NewEncoder(writer).Encode(vm.ProvisioningResponse{
				Progress:   100,
				Errors:     nil,
				Identifier: "TEST-IDENTIFIER",
				Queued:     false,
			})
			testhelper.AssertNoErr(t, err)
		})

		testhelper.Mux.HandleFunc("/api/vsphere/v1/provisioning/progress.json/TEST-IDENTIFIER", func(writer http.ResponseWriter, request *http.Request) {
			testhelper.TestMethod(t, request, http.MethodGet)

			err := json.NewEncoder(writer).Encode(progress.Progress{
				TaskIdentifier: "TEST-IDENTIFIER",
				Queued:         false,
				Progress:       100,
				VMIdentifier:   "VM-IDENTIFIER",
				Errors:         nil,
			})
			testhelper.AssertNoErr(t, err)
		})

		providerStatus := anxtypes.ProviderStatus{}
		ctx := createReconcileContext(reconcileContext{
			Machine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "TestMachine"},
			},
			Status:   &providerStatus,
			UserData: "",
			Config: resolvedConfig{
				VlanID:     "VLAN-ID",
				LocationID: "LOCATION-ID",
				TemplateID: "TEMPLATE-ID",
				Disks: []resolvedDisk{
					{
						RawDisk: anxtypes.RawDisk{
							Size: 5,
						},
					},
				},
				RawConfig: anxtypes.RawConfig{
					CPUs:   5,
					Memory: 5,
				},
			},
			ProviderData: &cloudprovidertypes.ProviderData{
				Update: func(m *clusterv1alpha1.Machine, mods ...cloudprovidertypes.MachineModifier) error {
					return nil
				},
			},
		})

		err := provisionVM(ctx, client)
		testhelper.AssertNoErr(t, err)
	})

	t.Run("Test is VM Provisioning", func(t *testing.T) {
		t.Parallel()
		providerStatus := anxtypes.ProviderStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ProvisionedType,
					Reason: "InProvisioning",
					Status: metav1.ConditionFalse,
				},
			},
		}
		ctx := createReconcileContext(reconcileContext{
			Status:       &providerStatus,
			UserData:     "",
			Config:       resolvedConfig{},
			ProviderData: nil,
		})

		condition := meta.FindStatusCondition(providerStatus.Conditions, ProvisionedType)
		condition.LastTransitionTime = metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
		testhelper.AssertEquals(t, true, isAlreadyProvisioning(ctx))

		condition.Reason = "Provisioned"
		condition.Status = metav1.ConditionTrue
		testhelper.AssertEquals(t, false, isAlreadyProvisioning(ctx))

		condition.Reason = "InProvisioning"
		condition.Status = metav1.ConditionFalse
		condition.LastTransitionTime = metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
		testhelper.AssertEquals(t, false, isAlreadyProvisioning(ctx))
		testhelper.AssertEquals(t, condition.Reason, "ReInitialising")
	})

	t.Run("Test getIPAddress", func(t *testing.T) {
		t.Parallel()
		providerStatus := &anxtypes.ProviderStatus{
			ReservedIP: "",
			IPState:    "",
		}
		ctx := createReconcileContext(reconcileContext{Status: providerStatus})

		t.Run("with unbound reserved IP", func(t *testing.T) {
			expectedIP := "8.8.8.8"
			providerStatus.ReservedIP = expectedIP
			providerStatus.IPState = anxtypes.IPStateUnbound
			reservedIP, err := getIPAddress(ctx, client)
			testhelper.AssertNoErr(t, err)
			testhelper.AssertEquals(t, expectedIP, reservedIP)
		})
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	// this generates a full config and allows hooking into it to e.g. remove a value
	hookableConfig := func(hook func(*anxtypes.RawConfig)) anxtypes.RawConfig {
		config := anxtypes.RawConfig{
			CPUs: 1,

			Memory: 2,

			Disks: []anxtypes.RawDisk{
				{Size: 5, PerformanceType: newConfigVarString("ENT6")},
			},

			Token:      newConfigVarString("test-token"),
			VlanID:     newConfigVarString("test-vlan"),
			LocationID: newConfigVarString("test-location"),
			TemplateID: newConfigVarString("test-template"),
		}

		if hook != nil {
			hook(&config)
		}

		return config
	}

	var configCases []ConfigTestCase
	configCases = append(configCases,
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Token.Value = "" }),
			Error:  errors.New("token is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.CPUs = 0 }),
			Error:  errors.New("cpu count is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Disks = []anxtypes.RawDisk{} }),
			Error:  errors.New("no disks configured"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.DiskSize = 10 }),
			Error:  ErrConfigDiskSizeAndDisks,
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Disks = append(c.Disks, anxtypes.RawDisk{Size: 10}) }),
			Error:  ErrMultipleDisksNotYetImplemented,
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Disks[0].Size = 0 }),
			Error:  errors.New("disk size is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Memory = 0 }),
			Error:  errors.New("memory size is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.LocationID.Value = "" }),
			Error:  errors.New("location id is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.TemplateID.Value = "" }),
			Error:  errors.New("template id is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.VlanID.Value = "" }),
			Error:  errors.New("vlan id is missing"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.DiskSize = 10; c.Disks = []anxtypes.RawDisk{} }),
			Error:  nil,
		},
		ConfigTestCase{
			Config: hookableConfig(nil),
			Error:  nil,
		},
	)

	provider := New(nil)
	for _, testCase := range getSpecsForValidationTest(t, configCases) {
		err := provider.Validate(testCase.Spec)
		if testCase.ExpectedError != nil {
			if !errors.Is(err, testCase.ExpectedError) {
				testhelper.AssertEquals(t, testCase.ExpectedError.Error(), err.Error())
			}
		} else {
			testhelper.AssertEquals(t, testCase.ExpectedError, err)
		}
	}
}

func TestEnsureConditions(t *testing.T) {
	t.Parallel()
	status := anxtypes.ProviderStatus{}

	ensureConditions(&status)

	condition := meta.FindStatusCondition(status.Conditions, ProvisionedType)
	if condition == nil {
		t.Fatal("condition should not be nil")
	}
	testhelper.AssertEquals(t, metav1.ConditionUnknown, condition.Status)
	testhelper.AssertEquals(t, "Initialising", condition.Reason)
}

func TestGetProviderStatus(t *testing.T) {
	t.Parallel()

	machine := &v1alpha1.Machine{}
	providerStatus := anxtypes.ProviderStatus{
		InstanceID: "InstanceID",
	}
	providerStatusJSON, err := json.Marshal(providerStatus)
	testhelper.AssertNoErr(t, err)
	machine.Status.ProviderStatus = &runtime.RawExtension{Raw: providerStatusJSON}

	returnedStatus := getProviderStatus(machine)

	testhelper.AssertEquals(t, "InstanceID", returnedStatus.InstanceID)

}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	machine := &v1alpha1.Machine{}
	providerStatus := anxtypes.ProviderStatus{
		InstanceID: "InstanceID",
	}
	providerStatusJSON, err := json.Marshal(providerStatus)
	testhelper.AssertNoErr(t, err)
	machine.Status.ProviderStatus = &runtime.RawExtension{Raw: providerStatusJSON}

	called := false
	err = updateMachineStatus(machine, providerStatus, func(paramMachine *v1alpha1.Machine, modifier ...cloudprovidertypes.MachineModifier) error {
		called = true
		testhelper.AssertEquals(t, machine, paramMachine)
		status := getProviderStatus(machine)
		testhelper.AssertEquals(t, status.InstanceID, providerStatus.InstanceID)
		return nil
	})

	testhelper.AssertEquals(t, true, called)
	testhelper.AssertNoErr(t, err)
}
