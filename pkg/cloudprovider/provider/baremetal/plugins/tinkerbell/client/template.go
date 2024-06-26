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

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Task struct {
	Name        string            `json:"name"`
	WorkerAddr  string            `json:"worker" yaml:"worker"`
	Actions     []Action          `json:"actions"`
	Volumes     []string          `json:"volumes,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// Action represents a workflow action.
type Action struct {
	Name        string            `json:"name,omitempty"`
	Image       string            `json:"image,omitempty"`
	Timeout     int64             `json:"timeout,omitempty"`
	Volumes     []string          `json:"volumes,omitempty"`
	Pid         string            `json:"pid,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Command     []string          `json:"command,omitempty"`
}
type Template struct {
	Version       string `yaml:"version"`
	Name          string `yaml:"name"`
	GlobalTimeout int64  `yaml:"global_timeout"`
	Tasks         []Task `yaml:"tasks"`
}

const fsType = "ext4"
const defaultInterpreter = "/bin/sh -c"
const hardwareDisk1 = "{{ index .Hardware.Disks 0 }}"

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

func (t *TemplateClient) Delete(ctx context.Context, namespacedName types.NamespacedName) error {
	template := &tinkv1alpha1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}
	// Create the Template object in the Tinkerbell cluster
	if err := t.tinkclient.Delete(ctx, template); err != nil {
		return fmt.Errorf("failed to delete Template in Tinkerbell cluster: %w", err)
	}

	return nil
}

// CreateTemplate creates a Tinkerbell Template in the Kubernetes cluster.
func (t *TemplateClient) CreateTemplate(ctx context.Context, namespacedName types.NamespacedName, osImageURL, hegelURL string) (*tinkv1alpha1.Template, error) {
	data, err := getTemplate(osImageURL, hegelURL)

	if err != nil {
		return nil, err
	}
	template := &tinkv1alpha1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: tinkv1alpha1.TemplateSpec{
			Data: &data, // templateData is a string containing the YAML definition.
		},
	}

	// Create the Template object in the Tinkerbell cluster
	if err := t.tinkclient.Create(ctx, template); err != nil {
		return nil, fmt.Errorf("failed to create Template in Tinkerbell cluster: %w", err)
	}

	return template, nil
}

func getTemplate(osImageURL, hegelURL string) (string, error) {
	actions := []Action{
		createWipeDiskAction(),
		createStreamUbuntuImageAction(hardwareDisk1, osImageURL),
		createGrowPartitionAction(hardwareDisk1),
		createCloudInitConfigAction(hardwareDisk1, hegelURL),
		createCloudInitIdentityAction(hardwareDisk1),
		createKexecAction(hardwareDisk1),
	}

	task := Task{
		Name:       "os-installation",
		WorkerAddr: "{{.device_1}}",
		Volumes:    []string{"/dev:/dev", "/dev/console:/dev/console", "/lib/firmware:/lib/firmware:ro"},
		Actions:    actions,
	}

	template := Template{
		Name:          "ubuntu",
		Version:       "0.1",
		GlobalTimeout: 1800,
		Tasks:         []Task{task},
	}
	// Marshal task to YAML
	yamlData, err := yaml.Marshal(template)
	if err != nil {
		return "", fmt.Errorf("error marshaling the template to YAML: %w", err)
	}

	return string(yamlData), nil
}

func createWipeDiskAction() Action {
	wipeScript := `apk add --no-cache util-linux
disks="{{ .Hardware.Disks }}"
disks=${disks:1:-1}
for disk in $disks; do
  for partition in $(ls ${disk}* 2>/dev/null); do
    if [ -b "${partition}" ]; then
      echo "Wiping ${partition}..."
      wipefs -af "${partition}"
    fi
  done
done
echo "All partitions on ${disks} have been wiped."
`
	return Action{
		Name:    "wipe-disk",
		Image:   "alpine:3.18",
		Timeout: 600,
		Command: []string{"/bin/sh", "-c", wipeScript},
	}
}

func createStreamUbuntuImageAction(destDisk, osImageURL string) Action {
	return Action{
		Name:    "stream-ubuntu-image",
		Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
		Timeout: 600,
		Environment: map[string]string{
			"DEST_DISK":  destDisk,
			"IMG_URL":    osImageURL,
			"COMPRESSED": "true",
		},
	}
}

func createGrowPartitionAction(destDisk string) Action {
	return Action{
		Name:    "grow-partition",
		Image:   "quay.io/tinkerbell-actions/cexec:v1.0.0",
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        destDisk + "3",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": defaultInterpreter,
			"CMD_LINE":            fmt.Sprintf("growpart %s 3 && resize2fs %s3", destDisk, destDisk),
		},
	}
}

func createCloudInitConfigAction(destDisk, metadataURL string) Action {
	// Use fmt.Sprintf to inject the variable into the string
	cloudInitConfiguration := fmt.Sprintf(`
datasource:
  Ec2:
    metadata_urls: ["%s"]
    strict_id: false
manage_etc_hosts: localhost
warnings:
  dsid_missing_source: off
`, metadataURL)

	return Action{
		Name:    "add-cloud-init-config",
		Image:   "quay.io/tinkerbell-actions/writefile:v1.0.0",
		Timeout: 90,
		Environment: map[string]string{
			"DEST_DISK": destDisk + "1",
			"FS_TYPE":   fsType,
			"DEST_PATH": "/etc/cloud/cloud.cfg.d/10_tinkerbell.cfg",
			"CONTENTS":  cloudInitConfiguration,
			"UID":       "0",
			"GID":       "0",
			"MODE":      "0644",
			"DIRMODE":   "0755",
		},
	}
}

func createCloudInitIdentityAction(destDisk string) Action {
	dsContent := `
datasource: Ec2
`
	return Action{
		Name:    "add-cloud-init-identity",
		Image:   "quay.io/tinkerbell-actions/writefile:v1.0.0",
		Timeout: 90,
		Environment: map[string]string{
			"DEST_DISK": destDisk + "1",
			"FS_TYPE":   fsType,
			"DEST_PATH": "/etc/cloud/ds-identify.cfg",
			"CONTENTS":  dsContent,
			"UID":       "0",
			"GID":       "0",
			"MODE":      "0644",
			"DIRMODE":   "0755",
		},
	}
}

func createKexecAction(destDisk string) Action {
	return Action{
		Name:    "kexec",
		Image:   "ghcr.io/jacobweinstock/waitdaemon:latest",
		Timeout: 90,
		Pid:     "host",
		Environment: map[string]string{
			"BLOCK_DEVICE": destDisk + "1",
			"IMAGE":        "quay.io/tinkerbell-actions/kexec:v1.0.0",
			"WAIT_SECONDS": "10",
		},
		Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock"},
	}
}
