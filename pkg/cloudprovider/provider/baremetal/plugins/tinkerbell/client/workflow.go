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
	"encoding/base64"
	"fmt"
	"time"

	tink "k8c.io/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/types"

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
func (w *WorkflowClient) CreateWorkflow(ctx context.Context, userData, templateRef string, hardware tink.Hardware) error {
	// Construct the Workflow object
	ifaceConfig := hardware.Spec.Interfaces[0].DHCP
	dnsNameservers := "1.1.1.1"

	for _, ns := range ifaceConfig.NameServers {
		dnsNameservers = ns
	}

	workflowName := fmt.Sprintf("%s-%s-%s", hardware.Name, templateRef, time.Now().Format("20060102150405"))
	workflow := &tinkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: hardware.Namespace,
			Labels: map[string]string{
				tink.HardwareRefLabel: hardware.Name,
			},
		},
		Spec: tinkv1alpha1.WorkflowSpec{
			TemplateRef: templateRef,
			HardwareRef: hardware.GetName(),
			HardwareMap: map[string]string{
				"device_1":          hardware.GetMACAddress(),
				"hardware_name":     hardware.GetName(),
				"cloud_init_script": base64.StdEncoding.EncodeToString([]byte(userData)),
				"interface_name":    ifaceConfig.IfaceName,
				"cidr":              convertNetmaskToCIDR(ifaceConfig.IP),
				"ns":                dnsNameservers,
				"default_route":     ifaceConfig.IP.Gateway,
			},
		},
	}

	// Create the Workflow in the cluster
	if err := w.tinkclient.Create(ctx, workflow); err != nil {
		return fmt.Errorf("failed to create the workflow: %w", err)
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

// CleanupWorkflows would delete all workflows that are assigned to a de-provisioned hardware, and they are in a pending
// state, to avoid the situation of re-running a workflow for a de-provisioned machine.
func (w *WorkflowClient) CleanupWorkflows(ctx context.Context, hardwareName, namespace string) error {
	workflows := &tinkv1alpha1.WorkflowList{}
	if err := w.tinkclient.List(ctx, workflows, &client.ListOptions{
		Namespace: namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			tink.HardwareRefLabel: hardwareName,
		}),
	}); err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to fetch workflows: %w", err)
	}

	for _, workflow := range workflows.Items {
		if workflow.Status.State == tinkv1alpha1.WorkflowStatePending ||
			workflow.Status.State == tinkv1alpha1.WorkflowStateTimeout {
			if err := w.tinkclient.Delete(ctx, &workflow); err != nil {
				if !kerrors.IsNotFound(err) {
					return fmt.Errorf("failed to delete workflow: %w", err)
				}
			}
		}
	}

	return nil
}
