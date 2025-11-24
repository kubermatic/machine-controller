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
	"testing"
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
