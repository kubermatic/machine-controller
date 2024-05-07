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

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkflowClient handles interactions with the Tinkerbell Workflows.
type TemplateClient struct {
	tinkclient client.Client
}

// NewTemplateClient creates a new client for managing Tinkerbell Templates.
func NewTemplateClient(k8sClient client.Client) *TemplateClient {
	return &TemplateClient{
		tinkclient: k8sClient,
	}
}

// CreateTemplate creates a Tinkerbell Template in the Kubernetes cluster.
func (t *TemplateClient) CreateTemplate(ctx context.Context) (*tinkv1alpha1.Template, error) {
	template := &tinkv1alpha1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ubuntu",
			Namespace: "default", // Adjust the namespace according to your setup
		},
		Spec: tinkv1alpha1.TemplateSpec{
			Data: &templateData, // templateData is a string containing the YAML definition.
		},
	}

	// Create the Template object in the Tinkerbell cluster
	if err := t.tinkclient.Create(ctx, template); err != nil {
		return nil, fmt.Errorf("failed to create Template in Tinkerbell cluster: %w", err)
	}

	fmt.Printf("Template %s created successfully\n", template.Name)
	return template, nil
}

var templateData = `
version: "0.1"
name: ubuntu
global_timeout: 1800
tasks:
  - name: "os installation"
    worker: "{{.device_1}}"
    volumes:
      - /dev:/dev
      - /dev/console:/dev/console
      - /lib/firmware:/lib/firmware:ro
    actions:
      - name: "stream ubuntu image"
        image: quay.io/tinkerbell/actions/image2disk:latest
        timeout: 600
        environment:
          DEST_DISK: {{ index .Hardware.Disks 0 }}
          IMG_URL: "http://$TINKERBELL_HOST_IP:8080/jammy-server-cloudimg-amd64.raw.gz"
          COMPRESSED: true
      # Add other actions here following the same pattern as above
`
