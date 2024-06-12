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
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HardwareClient manages Tinkerbell hardware resources across two clusters.
type HardwareClient struct {
	TinkerbellClient client.Client // Client for the Tinkerbell Cluster
}

// NewHardwareClient creates a new instance of HardwareClient.
func NewHardwareClient(tinkerbellClient client.Client) *HardwareClient {
	return &HardwareClient{
		//KubeClient:       kubeClient,
		TinkerbellClient: tinkerbellClient,
	}
}

// SelectAvailableHardware selects an available hardware from the given list of hardware references
// that has an empty ID.
func (h *HardwareClient) SelectAvailableHardware(ctx context.Context, hardwareRefs []types.NamespacedName) (*tinkv1alpha1.Hardware, error) {
	for _, ref := range hardwareRefs {
		var hardware tinkv1alpha1.Hardware
		if err := h.TinkerbellClient.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &hardware); err != nil {
			return nil, fmt.Errorf("failed to get hardware '%s' in namespace '%s': %w", ref.Name, ref.Namespace, err)
		}

		// Check if the ID is empty and return the hardware if it is
		if hardware.Spec.Metadata.Instance.ID == "" {
			return &hardware, nil // Found an unclaimed hardware
		}
	}

	return nil, fmt.Errorf("failed to get available hardware to provision")
}

// GetHardware selects an available hardware from the given list of hardware references
// that has an empty ID.
func (h *HardwareClient) GetHardware(ctx context.Context, hardwareRef types.NamespacedName) (*tinkv1alpha1.Hardware, error) {
	var hardware tinkv1alpha1.Hardware
	if err := h.TinkerbellClient.Get(ctx, client.ObjectKey{Namespace: hardwareRef.Namespace, Name: hardwareRef.Name}, &hardware); err != nil {
		return nil, fmt.Errorf("failed to get hardware '%s' in namespace '%s': %w", hardwareRef.Name, hardwareRef.Namespace, err)
	}

	// Check if the ID is empty and return the hardware if it is
	if hardware.Spec.Metadata.Instance.ID == "" {
		return &hardware, nil // Found an unclaimed hardware
	}

	return nil, fmt.Errorf("failed to get available hardware to provision")
}

// SetHardwareID sets the ID of a specified Hardware object.
func (h *HardwareClient) SetHardwareID(ctx context.Context, hardware *tinkv1alpha1.Hardware, newID string) error {
	// Set the new ID
	hardware.Spec.Metadata.Instance.ID = newID

	// Update the hardware object in the cluster
	if err := h.TinkerbellClient.Update(ctx, hardware); err != nil {
		return fmt.Errorf("failed to update hardware ID for '%s': %w", hardware.Name, err)
	}

	return nil
}

// CreateHardwareOnTinkCluster creates a hardware object on the Tinkerbell cluster.
func (h *HardwareClient) CreateHardwareOnTinkCluster(ctx context.Context, hardware *tinkv1alpha1.Hardware) error {
	// Set the namespace if it is not already specified
	if hardware.Namespace == "" {
		hardware.Namespace = "default"
	}

	hardware.ResourceVersion = ""
	// Create the hardware object on the Tinkerbell cluster
	if err := h.TinkerbellClient.Create(ctx, hardware); err != nil {
		return fmt.Errorf("failed to create hardware in Tinkerbell cluster: %w", err)
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

// SetHardwareUserData sets the User Data (cloud-init) of a specified Hardware object.
func (h *HardwareClient) SetHardwareUserData(ctx context.Context, hardware *tinkv1alpha1.Hardware, userdata string) error {
	// Set the new ID
	hardware.Spec.UserData = &userdata

	// Update the hardware object in the cluster
	if err := h.TinkerbellClient.Update(ctx, hardware); err != nil {
		return fmt.Errorf("failed to update hardware UserData for '%s': %w", hardware.Name, err)
	}

	return nil
}
