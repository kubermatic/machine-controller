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

	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/template"
)

type HardwareClient interface {
	Get(context.Context, string, string, string) (*hardware.Hardware, error)
	Delete(context.Context, string) error
	Create(context.Context, *hardware.Hardware) error
}

type TemplateClient interface {
	Get(context.Context, string, string) (*template.WorkflowTemplate, error)
	Create(context.Context, *template.WorkflowTemplate) error
}

type WorkflowClient interface {
	Create(context.Context, string, string) (string, error)
}
