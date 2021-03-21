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
	"fmt"

	"github.com/tinkerbell/tink/protos/template"
)

// Template client for Tinkerbell.
type Template struct {
	client template.TemplateServiceClient
}

// NewTemplateClient returns a Template client.
func NewTemplateClient(client template.TemplateServiceClient) *Template {
	return &Template{client: client}
}

// Get returns a Tinkerbell Template.
func (t *Template) Get(ctx context.Context, id, name string) (*template.WorkflowTemplate, error) {
	req := &template.GetRequest{}
	if id != "" {
		req.GetBy = &template.GetRequest_Id{Id: id}
	} else {
		req.GetBy = &template.GetRequest_Name{Name: name}
	}

	tinkTemplate, err := t.client.GetTemplate(ctx, req)
	if err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return nil, fmt.Errorf("template %w", ErrNotFound)
		}

		return nil, fmt.Errorf("getting template from Tinkerbell: %w", err)
	}

	return tinkTemplate, nil
}

// Update a Tinkerbell Template.
func (t *Template) Update(ctx context.Context, template *template.WorkflowTemplate) error {
	if _, err := t.client.UpdateTemplate(ctx, template); err != nil {
		return fmt.Errorf("updating template in Tinkerbefll: %w", err)
	}

	return nil
}

// Create a Tinkerbell Template.
func (t *Template) Create(ctx context.Context, template *template.WorkflowTemplate) error {
	resp, err := t.client.CreateTemplate(ctx, template)
	if err != nil {
		return fmt.Errorf("creating template in Tinkerbell: %w", err)
	}

	template.Id = resp.GetId()

	return nil
}

// Delete a Tinkerbell Template.
func (t *Template) Delete(ctx context.Context, id string) error {
	req := &template.GetRequest{
		GetBy: &template.GetRequest_Id{Id: id},
	}
	if _, err := t.client.DeleteTemplate(ctx, req); err != nil {
		if err.Error() == sqlErrorString || err.Error() == sqlErrorStringAlt {
			return fmt.Errorf("template %w", ErrNotFound)
		}

		return fmt.Errorf("deleting template from Tinkerbell: %w", err)
	}

	return nil
}
