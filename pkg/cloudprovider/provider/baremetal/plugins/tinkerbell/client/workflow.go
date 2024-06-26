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

	tink "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/types"

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkflowClient handles interactions with the Tinkerbell Workflows.
type WorkflowClient struct {
	tinkclient client.Client
}

// NewWorkflowClient creates a new client for managing Tinkerbell workflows.
func NewWorkflowClient(k8sClient client.Client) *WorkflowClient {
	return &WorkflowClient{
		tinkclient: k8sClient,
	}
}

// CreateWorkflow creates a new Tinkerbell Workflow resource in the cluster.
func (w *WorkflowClient) CreateWorkflow(ctx context.Context, workflowName, templateRef string, hardware tink.Hardware) error {
	// Construct the Workflow object
	workflow := &tinkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName + "-workflow",
			Namespace: hardware.Namespace,
		},
		Spec: tinkv1alpha1.WorkflowSpec{
			TemplateRef: templateRef,
			HardwareRef: hardware.GetName(),
			HardwareMap: map[string]string{
				"device_1": hardware.GetMACAddress(),
			},
		},
	}

	// Create the Workflow in the cluster
	if err := w.tinkclient.Create(ctx, workflow); err != nil {
		return fmt.Errorf("failed to create the workflow: %w", err)
	}
	return nil
}

// DeleteWorkflow deletes an existing Tinkerbell Workflow resource from the cluster.
func (w *WorkflowClient) DeleteWorkflow(ctx context.Context, name string, namespace string) error {
	workflow := &tinkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if err := w.tinkclient.Delete(ctx, workflow); err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}
	return nil
}

// GetWorkflow retrieves a Tinkerbell Workflow resource from the cluster.
func (w *WorkflowClient) GetWorkflow(ctx context.Context, name string, namespace string) (*tinkv1alpha1.Workflow, error) {
	workflow := &tinkv1alpha1.Workflow{}
	if err := w.tinkclient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, workflow); err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	return workflow, nil
}
