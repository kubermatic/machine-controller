/*
Copyright 2024 The Machine Controller Authors.

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

package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/google/go-cmp/cmp"

	"k8s.io/utils/ptr"

	azuretypes "k8c.io/machine-controller/sdk/cloudprovider/azure"
)

func TestVMSizeSupportsGen2(t *testing.T) {
	tests := []struct {
		name     string
		vmSize   string
		expected bool
	}{
		{
			name:     "Standard_F2s_v2 should support Gen2",
			vmSize:   "Standard_F2s_v2",
			expected: true,
		},
		{
			name:     "Standard_D2s_v3 should support Gen2",
			vmSize:   "Standard_D2s_v3",
			expected: true,
		},
		{
			name:     "Standard_E2s_v4 should support Gen2",
			vmSize:   "Standard_E2s_v4",
			expected: true,
		},
		{
			name:     "Standard_B2ms should support Gen2",
			vmSize:   "Standard_B2ms",
			expected: true,
		},
		{
			name:     "Standard_D2_v2 should support Gen2",
			vmSize:   "Standard_D2_v2",
			expected: true,
		},
		{
			name:     "Standard_A2 should not support Gen2",
			vmSize:   "Standard_A2",
			expected: false,
		},
		{
			name:     "Standard_D2 (old) should support Gen2",
			vmSize:   "Standard_D2",
			expected: true,
		},
		{
			name:     "lowercase Standard_f2s_v2 should support Gen2",
			vmSize:   "standard_f2s_v2",
			expected: true,
		},
		{
			name:     "Standard_NC6s_v3 should support Gen2",
			vmSize:   "Standard_NC6s_v3",
			expected: true,
		},
		{
			name:     "Standard_NC40ads_H100_v5 should support Gen2",
			vmSize:   "Standard_NC40ads_H100_v5",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vmSizeSupportsGen2(tt.vmSize)
			if result != tt.expected {
				t.Errorf("vmSizeSupportsGen2(%s) = %v, expected %v", tt.vmSize, result, tt.expected)
			}
		})
	}
}

func skuWithHyperVGenerations(generations string) compute.ResourceSku {
	return compute.ResourceSku{
		Capabilities: &[]compute.ResourceSkuCapabilities{
			{Name: to.StringPtr("HyperVGenerations"), Value: to.StringPtr(generations)},
		},
	}
}

func gen2SKU() compute.ResourceSku     { return skuWithHyperVGenerations("V1,V2") }
func gen2OnlySKU() compute.ResourceSku { return skuWithHyperVGenerations("V2") }
func gen1OnlySKU() compute.ResourceSku { return skuWithHyperVGenerations("V1") }
func skuWithoutGenCap() compute.ResourceSku {
	return compute.ResourceSku{
		Capabilities: &[]compute.ResourceSkuCapabilities{
			{Name: to.StringPtr("vCPUs"), Value: to.StringPtr("2")},
		},
	}
}

func TestValidateSecurityProfile(t *testing.T) {
	tests := []struct {
		name        string
		config      *config
		sku         compute.ResourceSku
		expectError bool
	}{
		{
			name:        "nil SecurityProfile passes",
			config:      &config{VMSize: "Standard_D2s_v3"},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "empty SecurityProfile (zero value) fails with empty securityType error",
			config: &config{
				VMSize:          "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "UEFI settings without securityType fails",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "invalid securityType ConfidentialVM fails",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesConfidentialVM,
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "garbage securityType fails",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypes("Nonsense"),
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "lowercase trustedlaunch fails (case-sensitive)",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypes("trustedlaunch"),
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "lowercase standard fails (case-sensitive)",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypes("standard"),
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "TrustedLaunch on non-Gen2 SKU fails",
			config: &config{
				VMSize: "Standard_A2",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
				},
			},
			sku:         gen1OnlySKU(),
			expectError: true,
		},
		{
			name: "TrustedLaunch on SKU without HyperVGenerations capability fails",
			config: &config{
				VMSize: "Standard_A2",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
				},
			},
			sku:         skuWithoutGenCap(),
			expectError: true,
		},
		{
			name: "TrustedLaunch on Gen2-only SKU passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
				},
			},
			sku:         gen2OnlySKU(),
			expectError: false,
		},
		{
			name: "TrustedLaunch on Gen2 SKU with no UefiSettings passes (Azure applies defaults)",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "TrustedLaunch with only secureBootEnabled set passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "TrustedLaunch with only vTpmEnabled set passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
					UefiSettings: &compute.UefiSettings{
						VTpmEnabled: ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "TrustedLaunch with secureBoot and vTpm both false passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(false),
						VTpmEnabled:       ptr.To(false),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "TrustedLaunch with both UEFI settings true passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: compute.SecurityTypesTrustedLaunch,
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(true),
						VTpmEnabled:       ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "Standard on Gen2 SKU passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "Standard on Gen1 SKU passes",
			config: &config{
				VMSize: "Standard_A2",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
				},
			},
			sku:         gen1OnlySKU(),
			expectError: false,
		},
		{
			name: "Standard with empty UefiSettings (both nil) passes",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
					UefiSettings: &compute.UefiSettings{},
				},
			},
			sku:         gen2SKU(),
			expectError: false,
		},
		{
			name: "Standard with secureBootEnabled fails",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "Standard with vTpmEnabled fails",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
					UefiSettings: &compute.UefiSettings{
						VTpmEnabled: ptr.To(true),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
		{
			name: "Standard with secureBootEnabled false fails (any non-nil pointer)",
			config: &config{
				VMSize: "Standard_D2s_v3",
				SecurityProfile: &compute.SecurityProfile{
					SecurityType: securityTypeStandard,
					UefiSettings: &compute.UefiSettings{
						SecureBootEnabled: ptr.To(false),
					},
				},
			},
			sku:         gen2SKU(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecurityProfile(context.Background(), tt.config, tt.sku)
			if (err != nil) != tt.expectError {
				t.Errorf("validateSecurityProfile() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestBuildSecurityProfile(t *testing.T) {
	tests := []struct {
		name     string
		raw      *azuretypes.SecurityProfile
		expected *compute.SecurityProfile
	}{
		{
			name:     "nil raw returns nil",
			raw:      nil,
			expected: nil,
		},
		{
			name:     "empty raw returns empty SecurityProfile (caught by validator)",
			raw:      &azuretypes.SecurityProfile{},
			expected: &compute.SecurityProfile{},
		},
		{
			name: "only securityType set",
			raw:  &azuretypes.SecurityProfile{SecurityType: "TrustedLaunch"},
			expected: &compute.SecurityProfile{
				SecurityType: compute.SecurityTypes("TrustedLaunch"),
			},
		},
		{
			name: "only secureBootEnabled set (no securityType) constructs UefiSettings, validator rejects later",
			raw:  &azuretypes.SecurityProfile{SecureBootEnabled: ptr.To(true)},
			expected: &compute.SecurityProfile{
				UefiSettings: &compute.UefiSettings{
					SecureBootEnabled: ptr.To(true),
				},
			},
		},
		{
			name: "only vTpmEnabled set constructs UefiSettings",
			raw:  &azuretypes.SecurityProfile{VTpmEnabled: ptr.To(true)},
			expected: &compute.SecurityProfile{
				UefiSettings: &compute.UefiSettings{
					VTpmEnabled: ptr.To(true),
				},
			},
		},
		{
			name: "both UEFI flags set, no securityType",
			raw: &azuretypes.SecurityProfile{
				SecureBootEnabled: ptr.To(true),
				VTpmEnabled:       ptr.To(false),
			},
			expected: &compute.SecurityProfile{
				UefiSettings: &compute.UefiSettings{
					SecureBootEnabled: ptr.To(true),
					VTpmEnabled:       ptr.To(false),
				},
			},
		},
		{
			name: "fully populated TrustedLaunch",
			raw: &azuretypes.SecurityProfile{
				SecurityType:      "TrustedLaunch",
				SecureBootEnabled: ptr.To(true),
				VTpmEnabled:       ptr.To(true),
			},
			expected: &compute.SecurityProfile{
				SecurityType: compute.SecurityTypes("TrustedLaunch"),
				UefiSettings: &compute.UefiSettings{
					SecureBootEnabled: ptr.To(true),
					VTpmEnabled:       ptr.To(true),
				},
			},
		},
		{
			name: "Standard with both UEFI flags (validator will reject) is still constructed",
			raw: &azuretypes.SecurityProfile{
				SecurityType:      "Standard",
				SecureBootEnabled: ptr.To(false),
				VTpmEnabled:       ptr.To(false),
			},
			expected: &compute.SecurityProfile{
				SecurityType: securityTypeStandard,
				UefiSettings: &compute.UefiSettings{
					SecureBootEnabled: ptr.To(false),
					VTpmEnabled:       ptr.To(false),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSecurityProfile(tt.raw)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("buildSecurityProfile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
