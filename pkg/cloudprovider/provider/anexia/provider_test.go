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
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/testhelper"
	"go.anx.io/go-anxcloud/pkg/api"
	"go.anx.io/go-anxcloud/pkg/api/mock"
	corev1 "go.anx.io/go-anxcloud/pkg/apis/core/v1"
	vspherev1 "go.anx.io/go-anxcloud/pkg/apis/vsphere/v1"
	"go.anx.io/go-anxcloud/pkg/client"
	anxclient "go.anx.io/go-anxcloud/pkg/client"
	"go.anx.io/go-anxcloud/pkg/core"
	"go.anx.io/go-anxcloud/pkg/ipam/address"
	"go.anx.io/go-anxcloud/pkg/vsphere/provisioning/progress"
	"go.anx.io/go-anxcloud/pkg/vsphere/provisioning/vm"
	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
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
	log := zap.NewNop().Sugar()

	a := mock.NewMockAPI()
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID-OLD-BUILD", Name: testTemplateName, Build: "b01"})
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID", Name: testTemplateName, Build: "b02"})
	a.FakeExisting(&vspherev1.Template{Identifier: "WRONG-TEMPLATE-NAME", Name: "Wrong Template Name", Build: "b02"})
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID-NO-NETWORK-CONFIG", Name: "no-network-config", Build: "b03"})
	a.FakeExisting(&vspherev1.Template{Identifier: "TEMPLATE-ID-ADDITIONAL-DISKS", Name: "additional-disks", Build: "b03"})

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
			{
				ReconcileContext: hookableReconcileContext("LOCATION-ID", "ADDITIONAL-DISKS", func(rc *reconcileContext) {
					rc.Config.Disks = append(rc.Config.Disks, resolvedDisk{
						RawDisk: anxtypes.RawDisk{
							Size: 10,
						},
						PerformanceType: "STD1",
					})
				}),

				AssertJSONBody: func(jsonBody jsonObject) {
					testhelper.AssertEquals(t, json.Number("5"), jsonBody["disk_gb"])
					testhelper.AssertJSONEquals(t, `[{"gb":10,"type":"STD1"}]`, jsonBody["additional_disks"])
				},
			},
		}

		testhelper.Mux.HandleFunc("/api/ipam/v1/address/reserve/ip/count.json", func(writer http.ResponseWriter, _ *http.Request) {
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

			err := provisionVM(ctx, log, client)
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
			templateID, err := provider.resolveTemplateID(context.Background(), a, testCase.config, "foo")
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
			Networks: []anxtypes.NetworkStatus{
				{
					Addresses: []anxtypes.NetworkAddressStatus{
						{
							ReservedIP: "",
							IPState:    "",
						},
					},
				},
			},
		}
		ctx := createReconcileContext(context.Background(), reconcileContext{Status: providerStatus})

		t.Run("with unbound reserved IP", func(t *testing.T) {
			expectedIP := "8.8.8.8"
			providerStatus.Networks[0].Addresses[0].ReservedIP = expectedIP
			providerStatus.Networks[0].Addresses[0].IPState = anxtypes.IPStateUnbound
			providerStatus.Networks[0].Addresses[0].IPProvisioningExpires = time.Now().Add(anxtypes.IPProvisioningExpires)
			reservedIP, err := getIPAddress(ctx, log, &resolvedNetwork{}, "Prefix-ID", &providerStatus.Networks[0].Addresses[0], client)
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
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Networks = []anxtypes.RawNetwork{} }),
			Error:  errors.New("no networks configured"),
		},
		ConfigTestCase{
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.VlanID.Value = "legacy VLAN-ID" }),
			Error:  ErrConfigVlanIDAndNetworks,
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
		err := provider.Validate(context.Background(), zap.NewNop().Sugar(), testCase.Spec)
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

	returnedStatus := getProviderStatus(zap.NewNop().Sugar(), machine)

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
	err = updateMachineStatus(machine, providerStatus, func(paramMachine *v1alpha1.Machine, _ ...cloudprovidertypes.MachineModifier) error {
		called = true
		testhelper.AssertEquals(t, machine, paramMachine)
		status := getProviderStatus(zap.NewNop().Sugar(), machine)
		testhelper.AssertEquals(t, status.InstanceID, providerStatus.InstanceID)
		return nil
	})

	testhelper.AssertEquals(t, true, called)
	testhelper.AssertNoErr(t, err)
}

func Test_anexiaErrorToTerminalError(t *testing.T) {
	forbiddenMockHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, err := w.Write([]byte(`{"error": {"code": 403}}`))
		testhelper.AssertNoErr(t, err)
	})

	unauthorizedMockHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{"error": {"code": 401}}`))
		testhelper.AssertNoErr(t, err)
	})

	legacyClientRun := func(url string) error {
		client, err := client.New(client.BaseURL(url), client.IgnoreMissingToken(), client.ParseEngineErrors(true))
		testhelper.AssertNoErr(t, err)
		_, err = core.NewAPI(client).Location().List(context.Background(), 1, 1, "", "")
		return err
	}

	apiClientRun := func(url string) error {
		client, err := api.NewAPI(api.WithClientOptions(
			client.BaseURL(url),
			client.IgnoreMissingToken(),
		))
		testhelper.AssertNoErr(t, err)
		return client.Get(context.Background(), &corev1.Location{Identifier: "foo"})
	}

	testCases := []struct {
		name        string
		mockHandler http.HandlerFunc
		run         func(url string) error
	}{
		{
			name:        "api client returns forbidden",
			mockHandler: forbiddenMockHandler,
			run:         apiClientRun,
		},
		{
			name:        "api client returns unauthorized",
			mockHandler: unauthorizedMockHandler,
			run:         apiClientRun,
		},
		{
			name:        "legacy client returns forbidden",
			mockHandler: forbiddenMockHandler,
			run:         legacyClientRun,
		},
		{
			name:        "legacy client returns unauthorized",
			mockHandler: unauthorizedMockHandler,
			run:         legacyClientRun,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			srv := httptest.NewServer(testCase.mockHandler)
			defer srv.Close()

			err := anexiaErrorToTerminalError(testCase.run(srv.URL), "foo")
			if ok, _, _ := cloudprovidererrors.IsTerminalError(err); !ok {
				t.Errorf("unexpected error %#v, expected TerminalError", err)
			}
		})
	}

	t.Run("api client 404 HTTPError shouldn't convert to TerminalError", func(t *testing.T) {
		err := api.NewHTTPError(http.StatusNotFound, "GET", &url.URL{}, errors.New("foo"))
		err = anexiaErrorToTerminalError(err, "foo")
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
			t.Errorf("unexpected error %#v, expected no TerminalError", err)
		}
	})

	t.Run("legacy api client unspecific ResponseError shouldn't convert to TerminalError", func(t *testing.T) {
		var err error = &client.ResponseError{}
		err = anexiaErrorToTerminalError(err, "foo")
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
			t.Errorf("unexpected error %#v, expected no TerminalError", err)
		}
	})
}
