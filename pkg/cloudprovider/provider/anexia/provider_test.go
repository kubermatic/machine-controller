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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/testhelper"
	"go.anx.io/go-anxcloud/pkg/api/mock"
	vspherev1 "go.anx.io/go-anxcloud/pkg/apis/vsphere/v1"
	anxclient "go.anx.io/go-anxcloud/pkg/client"
	"go.anx.io/go-anxcloud/pkg/ipam/address"
	"go.anx.io/go-anxcloud/pkg/vsphere/provisioning/progress"
	"go.anx.io/go-anxcloud/pkg/vsphere/provisioning/vm"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	TestIdentifier   = "TestIdent"
	testTemplateName = "test-template"
)

func TestAnexiaProvider(t *testing.T) {
	testhelper.SetupHTTP()
	client, server := anxclient.NewTestClient(nil, testhelper.Mux)

	a := mock.NewMockAPI()
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID-OLD-BUILD", Name: testTemplateName, Build: "b01"})
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID", Name: testTemplateName, Build: "b02"})
	a.FakeExisting(&vspherev1.Template{Identifier: "WRONG-TEMPLATE-NAME", Name: "Wrong Template Name", Build: "b02"})
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID-NO-NETWORK-CONFIG", Name: "no-network-config", Build: "b03"})

	t.Cleanup(func() {
		testhelper.TeardownHTTP()
		server.Close()
	})

	t.Run("Test provision VM", func(t *testing.T) {
		t.Parallel()

		testCases := []ProvisionVMTestCase{
			{
				// Provision a generic VM with some custom dns entries
				ReconcileContext: hookableReconcileContext("LOCATION-ID", "TEMPLATE-ID", func(rc *reconcileContext) {
					rc.ProviderConfig = &providerconfigtypes.Config{
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
					}
				}),
				AssertJSONBody: func(jsonBody jsonObject) {
					testhelper.AssertEquals(t, jsonBody["cpu_performance_type"], "performance")
					testhelper.AssertEquals(t, jsonBody["hostname"], "TestMachine")
					testhelper.AssertEquals(t, jsonBody["memory_mb"], json.Number("5"))

					testhelper.AssertEquals(t, jsonBody["dns1"], "1.1.1.1")
					_, exists := jsonBody["dns2"]
					testhelper.AssertEquals(t, exists, false)
					testhelper.AssertEquals(t, jsonBody["dns3"], "192.168.0.1")
					testhelper.AssertEquals(t, jsonBody["dns4"], "192.168.0.2")

					networkArray := jsonBody["network"].([]interface{})
					networkObject := networkArray[0].(jsonObject)
					testhelper.AssertEquals(t, networkObject["vlan"], "VLAN-ID")
					testhelper.AssertEquals(t, networkObject["nic_type"], "vmxnet3")
					testhelper.AssertEquals(t, networkObject["ips"].([]interface{})[0], "8.8.8.8")
				},
			},
			{
				// Provision a VM without any ProviderConfig
				ReconcileContext: hookableReconcileContext("LOCATION-ID", "TEMPLATE-ID-NO-NETWORK-CONFIG", func(rc *reconcileContext) {
					rc.ProviderConfig = &providerconfigtypes.Config{}
				}),
				AssertJSONBody: func(jsonBody jsonObject) {
					_, exists := jsonBody["dns1"]
					testhelper.AssertEquals(t, exists, false)
					_, exists = jsonBody["dns2"]
					testhelper.AssertEquals(t, exists, false)
					_, exists = jsonBody["dns3"]
					testhelper.AssertEquals(t, exists, false)
					_, exists = jsonBody["dns4"]
					testhelper.AssertEquals(t, exists, false)
				},
			},
		}

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

		for _, testCase := range testCases {
			templateID := testCase.ReconcileContext.Config.TemplateID
			locationID := testCase.ReconcileContext.Config.LocationID

			testhelper.Mux.HandleFunc(fmt.Sprintf("/api/vsphere/v1/provisioning/vm.json/%s/templates/%s", locationID, templateID), func(writer http.ResponseWriter, request *http.Request) {
				testhelper.TestMethod(t, request, http.MethodPost)
				var jsonBody jsonObject
				decoder := json.NewDecoder(request.Body)
				decoder.UseNumber()
				testhelper.AssertNoErr(t, decoder.Decode(&jsonBody))

				testCase.AssertJSONBody(jsonBody)

				err := json.NewEncoder(writer).Encode(vm.ProvisioningResponse{
					Progress:   100,
					Errors:     nil,
					Identifier: templateID,
					Queued:     false,
				})
				testhelper.AssertNoErr(t, err)
			})

			testhelper.Mux.HandleFunc(fmt.Sprintf("/api/vsphere/v1/provisioning/progress.json/%s", templateID), func(writer http.ResponseWriter, request *http.Request) {
				testhelper.TestMethod(t, request, http.MethodGet)

				err := json.NewEncoder(writer).Encode(progress.Progress{
					TaskIdentifier: templateID,
					Queued:         false,
					Progress:       100,
					VMIdentifier:   "VM-IDENTIFIER",
					Errors:         nil,
				})
				testhelper.AssertNoErr(t, err)
			})

			ctx := createReconcileContext(context.Background(), testCase.ReconcileContext)

			err := provisionVM(ctx, client)
			testhelper.AssertNoErr(t, err)
		}
	})

	t.Run("Test resolve template", func(t *testing.T) {
		t.Parallel()

		type testCase struct {
			config             anxtypes.RawConfig
			expectedError      string
			expectedTemplateID string
		}

		testCases := []testCase{
			// fail
			{
				// Template name does not exist
				config:        hookableConfig(func(c *anxtypes.RawConfig) { c.Template.Value = "non-existing-template-name" }),
				expectedError: "failed to retrieve named template",
			},
			{
				// Template build does not exist
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Template.Value = testTemplateName
					c.TemplateBuild.Value = "b42"
				}),
				expectedError: "failed to retrieve named template",
			},
			// pass
			{
				// With named template
				config:             hookableConfig(func(c *anxtypes.RawConfig) { c.Template.Value = testTemplateName; c.TemplateID.Value = "" }),
				expectedTemplateID: "TEMPLATE-ID",
			},
			{
				// With named template and not latest build
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Template.Value = testTemplateName
					c.TemplateBuild.Value = "b01"
				}),
				expectedTemplateID: "TEMPLATE-ID-OLD-BUILD",
			},
		}

		provider := New(nil).(*provider)
		for _, testCase := range testCases {
			templateID, err := resolveTemplateID(context.TODO(), a, testCase.config, provider.configVarResolver, "foo")
			if testCase.expectedError != "" {
				if err != nil {
					testhelper.AssertErr(t, err)
					testhelper.AssertEquals(t, true, strings.Contains(err.Error(), testCase.expectedError))
					continue
				}
			} else {
				testhelper.AssertNoErr(t, err)
				testhelper.AssertEquals(t, testCase.expectedTemplateID, templateID)
			}
		}
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
		ctx := createReconcileContext(context.Background(), reconcileContext{
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
		ctx := createReconcileContext(context.Background(), reconcileContext{Status: providerStatus})

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
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Token.Value = "" }),
			Error:  errors.New("token not set"),
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
		err := provider.Validate(context.Background(), testCase.Spec)
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
