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

	kerrors "k8s.io/apimachinery/pkg/api/errors"
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

const (
	fsType                      = "ext4"
	defaultInterpreter          = "/bin/sh -c"
	hardwareDisk1               = "{{ index .Hardware.Disks 0 }}"
	hardwareName                = "{{.hardware_name}}"
	ProvisionWorkerNodeTemplate = "provision-worker-node"
)

// TemplateClient handles interactions with the Tinkerbell Templates in the Tinkerbell cluster.
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

	if err := t.tinkclient.Delete(ctx, template); err != nil {
		return fmt.Errorf("failed to delete Template in Tinkerbell cluster: %w", err)
	}

	return nil
}

// CreateTemplate creates a Tinkerbell Template in the Kubernetes cluster.
func (t *TemplateClient) CreateTemplate(ctx context.Context, namespace, osImageURL string) error {
	template := &tinkv1alpha1.Template{}
	if err := t.tinkclient.Get(ctx, types.NamespacedName{
		Name:      ProvisionWorkerNodeTemplate,
		Namespace: namespace,
	}, template); err != nil {
		if kerrors.IsNotFound(err) {
			data, err := getTemplate(osImageURL)
			if err != nil {
				return err
			}

			template.Name = ProvisionWorkerNodeTemplate
			template.Namespace = namespace
			template.Spec = tinkv1alpha1.TemplateSpec{
				Data: &data, // templateData is a string containing the YAML definition.
			}

			// Create the Template object in the Tinkerbell cluster
			if err := t.tinkclient.Create(ctx, template); err != nil {
				return fmt.Errorf("failed to create Template in Tinkerbell cluster: %w", err)
			}

			return nil
		}

		return fmt.Errorf("failed to get template %s: %w", ProvisionWorkerNodeTemplate, err)
	}

	return nil
}

func getTemplate(osImageURL string) (string, error) {
	actions := []Action{
		createWipeDiskAction(),
		createStreamUbuntuImageAction(hardwareDisk1, osImageURL),
		createGrowPartitionAction(hardwareDisk1),
		createNetworkConfigAction(),
		configureCloudInitAction(),
		decodeCloudInitFile(hardwareName),
		createRebootAction(),
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
		Image:   "quay.io/tinkerbell/actions/cexec:c5bde803d9f6c90f1a9d5e06930d856d1481854c",
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ index .Hardware.Disks 0 }}3",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": defaultInterpreter,
			"CMD_LINE":            fmt.Sprintf("growpart %s 3 && resize2fs %s3", destDisk, destDisk),
		},
	}
}

func createNetworkConfigAction() Action {
	netplaneConfig := `
network:
  version: 2
  renderer: networkd
  ethernets:
    {{.interface_name}}:
      dhcp4: no
      addresses:
        - {{.cidr}}
      nameservers:
        addresses:
          - {{.ns}}
      routes:
      - to: default
        via: {{.default_route}}`
	return Action{
		Name:    "add-netplan-config",
		Image:   "quay.io/tinkerbell-actions/writefile:v1.0.0",
		Timeout: 90,
		Environment: map[string]string{
			"DEST_DISK": "{{ index .Hardware.Disks 0 }}3",
			"FS_TYPE":   fsType,
			"DEST_PATH": "/etc/netplan/config.yaml",
			"CONTENTS":  netplaneConfig,
			"UID":       "0",
			"GID":       "0",
			"MODE":      "0644",
			"DIRMODE":   "0755",
		},
	}
}

func configureCloudInitAction() Action {
	commands := `mkdir -p /var/lib/cloud/seed/nocloud && chmod 755 /var/lib/cloud/seed/nocloud
echo 'datasource_list: [ NoCloud ]' > /etc/cloud/cloud.cfg.d/01_ds-identify.cfg
echo '{{.cloud_init_script}}' > /tmp/{{.hardware_name}}-bootstrap-config
echo 'instance-id: {{.hardware_name}}' > /var/lib/cloud/seed/nocloud/meta-data
echo 'local-hostname: {{.hardware_name}}' >> /var/lib/cloud/seed/nocloud/meta-data
`

	return Action{
		Name:    "configure-cloud-init",
		Image:   "quay.io/tinkerbell-actions/cexec:v1.0.0",
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ index .Hardware.Disks 0 }}3",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": defaultInterpreter,
			"CMD_LINE":            commands,
		},
	}
}

func decodeCloudInitFile(hardwareName string) Action {
	return Action{
		Name:    "decode-cloud-init-file",
		Image:   "quay.io/tinkerbell/actions/cexec:latest",
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ index .Hardware.Disks 0 }}3",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": "/bin/sh -c",
			"CMD_LINE":            fmt.Sprintf("cat /tmp/%s-bootstrap-config | base64 -d > ''/var/lib/cloud/seed/nocloud/user-data'", hardwareName),
		},
	}
}

func createRebootAction() Action {
	return Action{
		Name:    "reboot-action",
		Image:   "ghcr.io/jacobweinstock/waitdaemon:0.1.1",
		Pid:     "host",
		Timeout: 90,
		Command: []string{"reboot"},
		Environment: map[string]string{
			"IMAGE":        "alpine",
			"WAIT_SECONDS": "10",
		},
		Volumes: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
		},
	}
}
