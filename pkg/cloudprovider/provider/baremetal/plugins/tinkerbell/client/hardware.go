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

package client

import (
	"context"
	"fmt"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	tbtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/types"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HardwareClient manages Tinkerbell hardware resources across two clusters.
type HardwareClient struct {
	TinkerbellClient client.Client
}

// NewHardwareClient creates a new instance of HardwareClient.
func NewHardwareClient(tinkerbellClient client.Client) *HardwareClient {
	return &HardwareClient{
		TinkerbellClient: tinkerbellClient,
	}
}

// GetHardware fetches a hardware object from the Tinkerbell cluster based on the hardware reference in the machine
// deployment object.
func (h *HardwareClient) GetHardware(ctx context.Context, hardwareRef types.NamespacedName) (*tinkv1alpha1.Hardware, error) {
	hardware := &tinkv1alpha1.Hardware{}
	if err := h.TinkerbellClient.Get(ctx, client.ObjectKey{Namespace: hardwareRef.Namespace, Name: hardwareRef.Name}, hardware); err != nil {
		return nil, fmt.Errorf("failed to get hardware '%s' in namespace '%s': %w", hardwareRef.Name, hardwareRef.Namespace, err)
	}

	return hardware, nil
}

// SetHardwareID sets the ID of a specified Hardware object.
func (h *HardwareClient) SetHardwareID(ctx context.Context, hardware *tinkv1alpha1.Hardware, newID string) error {
	if hardware.Spec.Metadata == nil {
		hardware.Spec.Metadata = &tinkv1alpha1.HardwareMetadata{}
	}

	if hardware.Spec.Metadata.Instance == nil {
		hardware.Spec.Metadata.Instance = &tinkv1alpha1.MetadataInstance{}
	}

	hardware.Spec.Metadata.Instance.ID = newID
	// Set the new ID
	hardware.Spec.Metadata.State = tbtypes.Staged
	if newID == "" {
		// Machine has been deprovisioned
		hardware.Spec.Metadata.State = tbtypes.Decommissioned
	}

	// Update the hardware object in the cluster
	if err := h.TinkerbellClient.Update(ctx, hardware); err != nil {
		return fmt.Errorf("failed to update hardware ID for '%s': %w", hardware.Name, err)
	}

	return nil
}

func (h *HardwareClient) GetHardwareWithID(ctx context.Context, uid string) (*tinkv1alpha1.Hardware, error) {
	// List all hardware in the cluster
	var hardwares tinkv1alpha1.HardwareList
	if err := h.TinkerbellClient.List(ctx, &hardwares); err != nil {
		return nil, fmt.Errorf("failed to list hardware: %w", err)
	}

	// Find the Hardware with the given ID
	var targetHardware tinkv1alpha1.Hardware
	for _, hw := range hardwares.Items {
		if hw.Spec.Metadata.Instance.ID == uid {
			targetHardware = hw
			return &targetHardware, nil
		}
	}

	return nil, errors.ErrInstanceNotFound
}
