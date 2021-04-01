/*
Copyright 2021 The Machine Controller Authors.

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
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
)

// Workflow client for Tinkerbell.
type Workflow struct {
	client         workflow.WorkflowServiceClient
	hardwareClient *Hardware
}

// NewWorkflowClient returns a Workflow client.
func NewWorkflowClient(client workflow.WorkflowServiceClient, hClient *Hardware) *Workflow {
	return &Workflow{client: client, hardwareClient: hClient}
}

// Get returns a Tinkerbell Workflow.
func (t *Workflow) Get(ctx context.Context, id string) (*workflow.Workflow, error) {
	tinkWorkflow, err := t.client.GetWorkflow(ctx, &workflow.GetRequest{Id: id})
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting workflow from Tinkerbell: %w", err)
	}

	return tinkWorkflow, nil
}

// GetMetadata returns the metadata for a given Tinkerbell Workflow.
func (t *Workflow) GetMetadata(ctx context.Context, id string) ([]byte, error) {
	verReq := &workflow.GetWorkflowDataRequest{WorkflowId: id}

	verResp, err := t.client.GetWorkflowDataVersion(ctx, verReq)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting workflow version from Tinkerbell: %w", err)
	}

	req := &workflow.GetWorkflowDataRequest{WorkflowId: id, Version: verResp.GetVersion()}

	resp, err := t.client.GetWorkflowMetadata(ctx, req)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting workflow metadata from Tinkerbell: %w", err)
	}

	return resp.GetData(), nil
}

// GetActions returns the actions for a given Tinkerbell Workflow.
func (t *Workflow) GetActions(ctx context.Context, id string) ([]*workflow.WorkflowAction, error) {
	req := &workflow.WorkflowActionsRequest{WorkflowId: id}

	resp, err := t.client.GetWorkflowActions(ctx, req)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting workflow actions from Tinkerbell: %w", err)
	}

	return resp.GetActionList(), nil
}

// GetEvents returns the events for a given Tinkerbell Workflow.
func (t *Workflow) GetEvents(ctx context.Context, id string) ([]*workflow.WorkflowActionStatus, error) {
	req := &workflow.GetRequest{Id: id}

	resp, err := t.client.ShowWorkflowEvents(ctx, req)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting workflow events from Tinkerbell: %w", err)
	}

	result := []*workflow.WorkflowActionStatus{}

	for {
		e, err := resp.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("getting workflow event from Tinkerbell: %w", err)
		}

		result = append(result, e)
	}

	return result, nil
}

// GetState returns the state for a given Tinkerbell Workflow.
func (t *Workflow) GetState(ctx context.Context, id string) (workflow.State, error) {
	req := &workflow.GetRequest{Id: id}

	resp, err := t.client.GetWorkflowContext(ctx, req)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return 0, fmt.Errorf("workflow %w", ErrNotFound)
		}

		return 0, fmt.Errorf("getting workflow state from Tinkerbell: %w", err)
	}

	currIndex := resp.GetCurrentActionIndex()
	total := resp.GetTotalNumberOfActions()
	currState := resp.GetCurrentActionState()

	switch {
	case total == 0:
		// If there are no actions, let's just call it pending
		return workflow.State_STATE_PENDING, nil
	case currIndex+1 == total:
		// If we are on the last action, just report it's state
		return currState, nil
	case currState != workflow.State_STATE_SUCCESS:
		// If the state of the last action is anything other than
		// success, just report it's state.
		return currState, nil
	default:
		// We are not on the last action, and the last action
		// was successful, we should report pending
		return workflow.State_STATE_PENDING, nil
	}
}

// Create a Tinkerbell Workflow.
func (t *Workflow) Create(ctx context.Context, templateID, hardwareID string) (string, error) {
	h, err := t.hardwareClient.Get(ctx, hardwareID, "", "")
	if err != nil {
		return "", err
	}

	hardwareString, err := HardwareToJSON(h)
	if err != nil {
		return "", err
	}

	req := &workflow.CreateRequest{
		Template: templateID,
		Hardware: hardwareString,
	}

	resp, err := t.client.CreateWorkflow(ctx, req)
	if err != nil {
		return "", fmt.Errorf("creating workflow in Tinkerbell: %w", err)
	}

	return resp.GetId(), nil
}

// Delete a Tinkerbell Workflow.
func (t *Workflow) Delete(ctx context.Context, id string) error {
	if _, err := t.client.DeleteWorkflow(ctx, &workflow.GetRequest{Id: id}); err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return fmt.Errorf("workflow %w", ErrNotFound)
		}

		return fmt.Errorf("deleting workflow from Tinkerbell: %w", err)
	}

	return nil
}

// HardwareToJSON converts Hardware to a string suitable for use in a
// Workflow Request for the raw Tinkerbell client.
func HardwareToJSON(h *hardware.Hardware) (string, error) {
	hardwareInterfaces := h.GetNetwork().GetInterfaces()
	hardwareInfo := make(map[string]string, len(hardwareInterfaces))

	for i, hi := range hardwareInterfaces {
		if mac := hi.GetDhcp().GetMac(); mac != "" {
			hardwareInfo[fmt.Sprintf("device_%d", i+1)] = mac
		}
	}

	hardwareJSON, err := json.Marshal(hardwareInfo)
	if err != nil {
		return "", fmt.Errorf("marshaling hardware info into json: %w", err)
	}

	return string(hardwareJSON), nil
}
