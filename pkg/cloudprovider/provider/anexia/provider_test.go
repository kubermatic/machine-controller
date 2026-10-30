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
	"strings"
	"testing"
	"time"

	"github.com/anexia/go-anxsdk"
	"github.com/anexia/go-anxsdk/paging"
	anxsdkcommon "github.com/anexia/go-anxsdk/v1/common"
	"github.com/anexia/go-anxsdk/v1/ipam"
	"github.com/anexia/go-anxsdk/v1/vsphere"
	"github.com/gophercloud/gophercloud/testhelper"
	"go.uber.org/zap"

	cloudprovidererrors "k8c.io/machine-controller/pkg/cloudprovider/errors"
	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
	providerconfigtypes "k8c.io/machine-controller/sdk/providerconfig"
	"k8c.io/machine-controller/sdk/providerconfig/configvar"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	TestIdentifier   = "TestIdent"
	testTemplateName = "test-template"
)

func TestAnexiaProvider(t *testing.T) {
	testhelper.SetupHTTP()
	server := httptest.NewServer(testhelper.Mux)
	sdkClient := anxsdk.NewClient(anxsdk.WithBaseURL(server.URL), anxsdk.WithHTTPClient(server.Client()))
	addressClient := sdkClient.V1().Ipam().Addresses()
	log := zap.NewNop().Sugar()

	testhelper.Mux.HandleFunc("/api/vsphere/v1/provisioning/templates.json/foo/templates", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(writer).Encode([]vsphere.TemplateResponse{
			{ID: "TEMPLATE-ID-OLD-BUILD", Name: testTemplateName, Build: "b01"},
			{ID: "TEMPLATE-ID", Name: testTemplateName, Build: "b02"},
			{ID: "WRONG-TEMPLATE-NAME", Name: "Wrong Template Name", Build: "b02"},
			{ID: "TEMPLATE-ID-NO-NETWORK-CONFIG", Name: "no-network-config", Build: "b03"},
			{ID: "TEMPLATE-ID-ADDITIONAL-DISKS", Name: "additional-disks", Build: "b03"},
		})
		testhelper.AssertNoErr(t, err)
	})

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
					rc.UserData = "#!/usr/bin/bash\n echo asdf"
					rc.ProviderConfig = &providerconfigtypes.Config{
						Network: &providerconfigtypes.NetworkConfig{
							DNS: providerconfigtypes.DNSConfig{
								Servers: []string{
									"1.1.1.1",
									"192.168.0.1",
									"192.168.0.2",
									"192.168.0.3",
								},
							},
						},
					}
				}),
				AssertJSONBody: func(jsonBody jsonObject) {
					testhelper.AssertEquals(t, jsonBody["cpu_performance_type"], "enterprise")
					testhelper.AssertEquals(t, jsonBody["disk_type"], "ENT4")
					testhelper.AssertEquals(t, jsonBody["hostname"], testMachineName)
					testhelper.AssertEquals(t, jsonBody["memory_mb"], json.Number("5"))

					testhelper.AssertEquals(t, jsonBody["dns1"], "1.1.1.1")
					testhelper.AssertEquals(t, jsonBody["dns2"], "192.168.0.1")
					testhelper.AssertEquals(t, jsonBody["dns3"], "192.168.0.2")
					testhelper.AssertEquals(t, jsonBody["dns4"], "192.168.0.3")

					networkArray := jsonBody["network"].([]any)
					networkObject := networkArray[0].(jsonObject)
					testhelper.AssertEquals(t, networkObject["vlan"], "VLAN-ID")
					testhelper.AssertEquals(t, networkObject["nic_type"], "virtio")
					testhelper.AssertEquals(t, networkObject["ips"].([]any)[0], testPublicIPv4)
					testhelper.AssertEquals(t, jsonBody["script"], "IyEvdXNyL2Jpbi9iYXNoCiBlY2hvIGFzZGY=") // user-data script as base64
					testhelper.AssertEquals(t, jsonBody["ssh"], "ssh-ed25519 AAAAC3Nza test1\nssh-rsa AAAAB3NzaC1yc test2")
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
			{
				// Provision a generic VM with an increased bandwidth limit
				ReconcileContext: hookableReconcileContext("LOCATION-ID", "INCREASED-BANDWIDTH-LIMIT", func(rc *reconcileContext) {
					rc.Config.Networks[0].BandwidthLimit = 10000
				}),
				AssertJSONBody: func(jsonBody jsonObject) {
					networkArray := jsonBody["network"].([]any)
					networkObject := networkArray[0].(jsonObject)
					testhelper.AssertEquals(t, json.Number("10000"), networkObject["bandwidth_limit"])
				},
			},
			{
				// Provision a generic VM with an availability zone
				ReconcileContext: hookableReconcileContext("LOCATION-ID", "SET-AVAILABILITY-ZONE", func(rc *reconcileContext) {
					rc.Config.AvailabilityZone = "zone"
				}),
				AssertJSONBody: func(jsonBody jsonObject) {
					zone := jsonBody["availability_zone"].(string)
					//networkObject := networkArray[0].(jsonObject)
					testhelper.AssertEquals(t, "zone", zone)
				},
			},
		}

		testhelper.Mux.HandleFunc("/api/ipam/v1/address/reserve/ip/count.json", func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(writer).Encode(paging.PagedResponse[ipam.AddressReserveResponseItem]{
				Data: []ipam.AddressReserveResponseItem{
					{
						Identifier: testIPIdentifier,
						Text:       testPublicIPv4,
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

				writer.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(writer).Encode(vsphere.ProvisioningResponse{
					Progress:       100,
					Errors:         nil,
					TaskIdentifier: templateID,
					Queued:         false,
				})
				testhelper.AssertNoErr(t, err)
			})

			testhelper.Mux.HandleFunc(fmt.Sprintf("/api/vsphere/v1/provisioning/progress.json/%s", templateID), func(writer http.ResponseWriter, request *http.Request) {
				testhelper.TestMethod(t, request, http.MethodGet)

				writer.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(writer).Encode(vsphere.ProvisioningProgress{
					TaskIdentifier: templateID,
					Queued:         false,
					Progress:       100,
					VMIdentifier:   "VM-IDENTIFIER",
					Errors:         nil,
					Status:         vsphere.ProvisioningStatusSuccess,
				})
				testhelper.AssertNoErr(t, err)
			})

			err := provisionVM(context.Background(), testCase.ReconcileContext, log, sdkClient)
			testhelper.AssertNoErr(t, err)
		}
	})

	t.Run("Test resolve network", func(t *testing.T) {
		t.Parallel()

		type testCase struct {
			config          anxtypes.RawConfig
			expectedError   string
			expectedNetwork []resolvedNetwork
		}

		testCases := []testCase{
			{
				// Failing to parse should mention the reason
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:         providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs:      []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
							BandwidthLimit: 19,
						},
					}
				}),
				expectedError:   "failed to parse bandwidth limit",
				expectedNetwork: []resolvedNetwork{},
			},
			{
				// Without Bandwidth specified
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:    providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs: []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
						},
					}
				}),
				expectedError: "",
				expectedNetwork: []resolvedNetwork{
					{
						VlanID:         "17825213",
						Prefixes:       []string{testPrefixID},
						BandwidthLimit: 0,
					},
				},
			},
			{
				// With one valid network
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:         providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs:      []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
							BandwidthLimit: 10000,
						},
					}
				}),
				expectedError: "",
				expectedNetwork: []resolvedNetwork{
					{
						VlanID:         "17825213",
						Prefixes:       []string{testPrefixID},
						BandwidthLimit: 10000,
					},
				},
			},
		}

		provider := New(configvar.NewResolver(context.Background(), fake.NewClientBuilder().Build())).(*provider)
		for _, testCase := range testCases {
			resolvedNetworks, err := provider.resolveNetworkConfig(log, testCase.config)
			if testCase.expectedError != "" {
				testhelper.AssertErr(t, err)
				testhelper.AssertEquals(t, true, strings.Contains(err.Error(), testCase.expectedError))
				continue
			} else {
				testhelper.AssertNoErr(t, err)
				for ni, network := range *resolvedNetworks {
					testhelper.AssertEquals(t, testCase.expectedNetwork[ni].VlanID, network.VlanID)
					for pi, prefix := range network.Prefixes {
						testhelper.AssertEquals(t, testCase.expectedNetwork[ni].Prefixes[pi], prefix)
					}
					testhelper.AssertEquals(t, testCase.expectedNetwork[ni].BandwidthLimit, network.BandwidthLimit)
				}
			}
		}
	})

	t.Run("Test resolve network", func(t *testing.T) {
		t.Parallel()

		type testCase struct {
			config                        anxtypes.RawConfig
			expectedError                 string
			expectedNetworkBandwidthLimit int
			expectedNetwork               []resolvedNetwork
		}

		testCases := []testCase{
			{
				// Failing to parse should mention the reason
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:         providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs:      []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
							BandwidthLimit: 19,
						},
					}
				}),
				expectedError:   "failed to parse bandwidth limit",
				expectedNetwork: []resolvedNetwork{},
			},
			{
				// Without Bandwidth specified
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:    providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs: []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
						},
					}
				}),
				expectedError: "",
				expectedNetwork: []resolvedNetwork{
					{
						VlanID:         "17825213",
						Prefixes:       []string{testPrefixID},
						BandwidthLimit: 0,
					},
				},
			},
			{
				// With one valid network
				config: hookableConfig(func(c *anxtypes.RawConfig) {
					c.Networks = []anxtypes.RawNetwork{
						{
							VlanID:         providerconfigtypes.ConfigVarString{Value: "17825213"},
							PrefixIDs:      []providerconfigtypes.ConfigVarString{{Value: testPrefixID}},
							BandwidthLimit: 10000,
						},
					}
				}),
				expectedError: "",
				expectedNetwork: []resolvedNetwork{
					{
						VlanID:         "17825213",
						Prefixes:       []string{testPrefixID},
						BandwidthLimit: 10000,
					},
				},
			},
		}

		provider := New(configvar.NewResolver(context.Background(), fake.NewClientBuilder().Build())).(*provider)
		for _, testCase := range testCases {
			resolvedNetworks, err := provider.resolveNetworkConfig(log, testCase.config)
			if testCase.expectedError != "" {
				testhelper.AssertErr(t, err)
				testhelper.AssertEquals(t, true, strings.Contains(err.Error(), testCase.expectedError))
				continue
			} else {
				testhelper.AssertNoErr(t, err)
				for ni, network := range *resolvedNetworks {
					testhelper.AssertEquals(t, testCase.expectedNetwork[ni].VlanID, network.VlanID)
					for pi, prefix := range network.Prefixes {
						testhelper.AssertEquals(t, testCase.expectedNetwork[ni].Prefixes[pi], prefix)
					}
					testhelper.AssertEquals(t, testCase.expectedNetwork[ni].BandwidthLimit, network.BandwidthLimit)
				}
			}
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

		provider := New(configvar.NewResolver(context.Background(), fake.NewClientBuilder().Build())).(*provider)
		for _, testCase := range testCases {
			templateID, err := provider.resolveTemplateID(context.Background(), sdkClient.V1().VSphere().Provisioning(), testCase.config, "foo")
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
		reconcileCtx := reconcileContext{
			Status:       &providerStatus,
			UserData:     "",
			Config:       resolvedConfig{},
			ProviderData: nil,
		}

		condition := meta.FindStatusCondition(providerStatus.Conditions, ProvisionedType)
		condition.LastTransitionTime = metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
		testhelper.AssertEquals(t, true, isAlreadyProvisioning(reconcileCtx))

		condition.Reason = "Provisioned"
		condition.Status = metav1.ConditionTrue
		testhelper.AssertEquals(t, false, isAlreadyProvisioning(reconcileCtx))

		condition.Reason = "InProvisioning"
		condition.Status = metav1.ConditionFalse
		condition.LastTransitionTime = metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
		testhelper.AssertEquals(t, false, isAlreadyProvisioning(reconcileCtx))
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
		reconcileCtx := reconcileContext{Status: providerStatus}

		t.Run("with unbound reserved IP", func(t *testing.T) {
			expectedIP := testPublicIPv4
			providerStatus.Networks[0].Addresses[0].ReservedIP = expectedIP
			providerStatus.Networks[0].Addresses[0].IPState = anxtypes.IPStateUnbound
			providerStatus.Networks[0].Addresses[0].IPProvisioningExpires = time.Now().Add(anxtypes.IPProvisioningExpires)
			reservedIP, err := getIPAddress(context.Background(), reconcileCtx, log, &resolvedNetwork{}, "Prefix-ID", &providerStatus.Networks[0].Addresses[0], addressClient)
			testhelper.AssertNoErr(t, err)
			testhelper.AssertEquals(t, expectedIP, reservedIP)
		})
	})
}

func isAlreadyProvisioning(reconcileContext reconcileContext) bool {
	status := reconcileContext.Status
	condition := meta.FindStatusCondition(status.Conditions, ProvisionedType)
	lastChange := condition.LastTransitionTime.Time
	const reasonInProvisioning = "InProvisioning"
	if condition.Reason == reasonInProvisioning && time.Since(lastChange) > 5*time.Minute {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:    ProvisionedType,
			Reason:  "ReInitialising",
			Message: "Could not find ongoing VM provisioning",
			Status:  metav1.ConditionFalse,
		})
	}

	return condition.Status == metav1.ConditionFalse && condition.Reason == reasonInProvisioning
}

func TestValidate(t *testing.T) {
	t.Parallel()

	configCases := []ConfigTestCase{
		{
			Name:   "no cpu count",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.CPUs = 0 }),
			Error:  errors.New("cpu count is missing"),
		},
		{
			Name:   "no disk size",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.DiskSize = 0 }),
			Error:  errors.New("disk size is missing"),
		},
		{
			Name:   "invalid disk performance type",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.DiskPerformanceType = "" }),
			Error:  vsphere.ErrInvalidDiskType,
		},
		{
			Name:   "wrong cpu performance type",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.CPUPerformanceType = "superfast" }),
			Error:  vsphere.ErrInvalidCPUPerformanceType,
		},
		{
			Name:   "no disk size for additional disk disk",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Disks[0].Size = 0 }),
			Error:  errors.New("disk size for disk 0 is missing"),
		},
		{
			Name:   "no disk performance type for additional disk disk",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Disks[0].PerformanceType.Value = "" }),
			Error:  vsphere.ErrInvalidDiskType,
		},
		{
			Name:   "no memory",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Memory = 0 }),
			Error:  errors.New("memory size is missing"),
		},
		{
			Name:   "no location id",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.LocationID.Value = "" }),
			Error:  errors.New("location id is missing"),
		},
		{
			Name:   "no networks",
			Config: hookableConfig(func(c *anxtypes.RawConfig) { c.Networks = []anxtypes.RawNetwork{} }),
			Error:  errors.New("no networks configured"),
		},
		{
			Name: "combined",
			Config: hookableConfig(func(c *anxtypes.RawConfig) {
				c.CPUs = 0
				c.Memory = 0
			}),
			Error: errors.Join(errors.New("cpu count is missing"), errors.New("memory size is missing")),
		},
		{
			Name:   "default is valid",
			Config: hookableConfig(nil),
			Error:  nil,
		},
	}

	provider := New(configvar.NewResolver(context.Background(), fake.NewClientBuilder().Build()))
	for _, testCase := range getSpecsForValidationTest(t, configCases) {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			err := provider.Validate(context.Background(), zap.NewNop().Sugar(), testCase.Spec)
			if testCase.ExpectedError != nil {
				if !errors.Is(err, testCase.ExpectedError) {
					testhelper.AssertEquals(t, testCase.ExpectedError.Error(), err.Error())
				}
			} else {
				testhelper.AssertEquals(t, testCase.ExpectedError, err)
			}
		})
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

	machine := &clusterv1alpha1.Machine{}
	providerStatus := anxtypes.ProviderStatus{
		InstanceID: "InstanceID",
	}
	providerStatusJSON, err := json.Marshal(providerStatus)
	testhelper.AssertNoErr(t, err)
	machine.Status.ProviderStatus = &runtime.RawExtension{Raw: providerStatusJSON}

	returnedStatus, err := getProviderStatus(zap.NewNop().Sugar(), machine)
	testhelper.AssertNoErr(t, err)

	testhelper.AssertEquals(t, "InstanceID", returnedStatus.InstanceID)
}

func TestGetProviderStatusCorrupt(t *testing.T) {
	t.Parallel()

	machine := &clusterv1alpha1.Machine{}
	machine.Status.ProviderStatus = &runtime.RawExtension{Raw: []byte("{not json")}

	// A corrupt status must not be silently reported as an empty status - that
	// would make the controller provision a second VM for this Machine.
	_, err := getProviderStatus(zap.NewNop().Sugar(), machine)
	testhelper.AssertErr(t, err)
}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	machine := &clusterv1alpha1.Machine{}
	providerStatus := anxtypes.ProviderStatus{
		InstanceID: "InstanceID",
	}
	providerStatusJSON, err := json.Marshal(providerStatus)
	testhelper.AssertNoErr(t, err)
	machine.Status.ProviderStatus = &runtime.RawExtension{Raw: providerStatusJSON}

	called := false
	err = updateMachineStatus(machine, providerStatus, func(paramMachine *clusterv1alpha1.Machine, _ ...cloudprovidertypes.MachineModifier) error {
		called = true
		testhelper.AssertEquals(t, machine, paramMachine)
		status, err := getProviderStatus(zap.NewNop().Sugar(), machine)
		testhelper.AssertNoErr(t, err)
		testhelper.AssertEquals(t, status.InstanceID, providerStatus.InstanceID)
		return nil
	})

	testhelper.AssertEquals(t, true, called)
	testhelper.AssertNoErr(t, err)
}

func Test_wrapAnexiaError(t *testing.T) {
	t.Run("go-anxsdk 403 APIError should convert to TerminalError", func(t *testing.T) {
		var err error = &anxsdkcommon.APIError{StatusCode: http.StatusForbidden}
		err = wrapAnexiaError(err, "foo")
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); !ok {
			t.Errorf("unexpected error %#v, expected TerminalError", err)
		}
	})

	t.Run("go-anxsdk 401 APIError should convert to TerminalError", func(t *testing.T) {
		var err error = &anxsdkcommon.APIError{StatusCode: http.StatusUnauthorized}
		err = wrapAnexiaError(err, "foo")
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); !ok {
			t.Errorf("unexpected error %#v, expected TerminalError", err)
		}
	})

	t.Run("go-anxsdk 404 APIError should convert to NotFoundError", func(t *testing.T) {
		var err error = &anxsdkcommon.APIError{StatusCode: http.StatusNotFound}
		err = wrapAnexiaError(err, "foo")
		if ok := cloudprovidererrors.IsNotFound(err); !ok {
			t.Errorf("unexpected error %#v, expected ErrInstanceNotFound", err)
		}
	})

	t.Run("go-anxsdk unspecific APIError shouldn't convert to TerminalError", func(t *testing.T) {
		var err error = &anxsdkcommon.APIError{StatusCode: http.StatusInternalServerError}
		err = wrapAnexiaError(err, "foo")
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
			t.Errorf("unexpected error %#v, expected no TerminalError", err)
		}
	})
}
