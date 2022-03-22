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
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/utils"
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
		ctx := utils.CreateReconcileContext(utils.ReconcileContext{
			Machine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "TestMachine"},
			},
			Status:   &providerStatus,
			UserData: "",
			Config:   &anxtypes.Config{},

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
		ctx := utils.CreateReconcileContext(utils.ReconcileContext{
			Machine: &v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "TestMachine"},
			},
			Status:   &providerStatus,
			UserData: "",
			Config: &anxtypes.Config{
				VlanID:     "VLAN-ID",
				LocationID: "LOCATION-ID",
				TemplateID: "TEMPLATE-ID",
				CPUs:       5,
				Memory:     5,
				DiskSize:   5,
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
		ctx := utils.CreateReconcileContext(utils.ReconcileContext{
			Status:       &providerStatus,
			UserData:     "",
			Config:       nil,
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
		ctx := utils.CreateReconcileContext(utils.ReconcileContext{Status: providerStatus})

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

	var configCases []ConfigTestCase
	configCases = append(configCases,
		ConfigTestCase{
			Config: anxtypes.RawConfig{},
			Error:  errors.New("token is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN")},
			Error:  errors.New("cpu count is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1},
			Error:  errors.New("disk size is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1, DiskSize: 5},
			Error:  errors.New("memory size is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1, DiskSize: 5, Memory: 5},
			Error:  errors.New("location id is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1, DiskSize: 5, Memory: 5,
				LocationID: newConfigVarString("TLID")},
			Error: errors.New("template id is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1, DiskSize: 5, Memory: 5,
				LocationID: newConfigVarString("LID"), TemplateID: newConfigVarString("TID")},
			Error: errors.New("vlan id is missing"),
		},
		ConfigTestCase{
			Config: anxtypes.RawConfig{Token: newConfigVarString("TEST-TOKEN"), CPUs: 1, DiskSize: 5, Memory: 5,
				LocationID: newConfigVarString("LID"), TemplateID: newConfigVarString("TID"), VlanID: newConfigVarString("VLAN")},
			Error: nil,
		},
	)

	provider := New(nil)
	for _, testCase := range getSpecsForValidationTest(t, configCases) {
		err := provider.Validate(testCase.Spec)
		if testCase.ExpectedError != nil {
			testhelper.AssertEquals(t, testCase.ExpectedError.Error(), err.Error())
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
