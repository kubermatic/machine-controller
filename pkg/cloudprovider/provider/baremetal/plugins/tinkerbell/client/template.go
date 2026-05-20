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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	PartitionNumber             = "{{.partition_number}}"
	OSImageURL                  = "{{.os_image}}"

	// Container images mirrored from upstream registries to quay.io/kubermatic-mirror.
	// Digests are pinned in hack/mirror-images.yaml. Update these constants together
	// with the YAML; the post-machine-controller-mirror-images postsubmit pushes the
	// new digest on merge.
	imageAlpine                  = "quay.io/kubermatic-mirror/images/alpine@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11"
	imageTinkerbellImage2Disk    = "quay.io/kubermatic-mirror/images/tinkerbell-actions/image2disk@sha256:5584faf90ca13e30b35a34423364468b109807eccafaab5820d3f549e6c25240"
	imageTinkerbellCexecPinned   = "quay.io/kubermatic-mirror/images/tinkerbell/actions/cexec-pinned@sha256:8ee8cae95c8762847ced09370fc5ecc9853fc83a26a3e4242f3f6efa813841fc"
	imageTinkerbellWriteFile     = "quay.io/kubermatic-mirror/images/tinkerbell-actions/writefile@sha256:30899f69c1710eedeccede260fa2e840a5775335874d7fb72b7a260d4adac620"
	imageTinkerbellActionsCexec  = "quay.io/kubermatic-mirror/images/tinkerbell-actions/cexec@sha256:57f775a8cbeda334221edd890e715310f367b3436280a52f15ac698de4784376"
	imageTinkerbellCexecResolved = "quay.io/kubermatic-mirror/images/tinkerbell/actions/cexec-latest-resolved@sha256:86d490ea9d5a27d0d11c1df812fdec49877e58e5076c4d5c0417d1a6dac530a9"
	imageWaitDaemon              = "quay.io/kubermatic-mirror/images/jacobweinstock/waitdaemon@sha256:907249ea6a9de0225f8aa583d9d8a92a5b77a89ab83d01b607a0051e0913dea5"
)

// TemplateClient handles interactions with the Tinkerbell Templates in the Tinkerbell cluster.
type TemplateClient struct {
	tinkclient ctrlruntimeclient.Client
}

// NewTemplateClient creates a new client for managing Tinkerbell Templates.
func NewTemplateClient(k8sClient ctrlruntimeclient.Client) *TemplateClient {
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
func (t *TemplateClient) CreateTemplate(ctx context.Context, namespace string) error {
	template := &tinkv1alpha1.Template{}
	if err := t.tinkclient.Get(ctx, types.NamespacedName{
		Name:      ProvisionWorkerNodeTemplate,
		Namespace: namespace,
	}, template); err != nil {
		if apierrors.IsNotFound(err) {
			data, err := getTemplate(OSImageURL)
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
		Image:   imageAlpine,
		Timeout: 600,
		Command: []string{"/bin/sh", "-c", wipeScript},
	}
}

func createStreamUbuntuImageAction(destDisk, osImageURL string) Action {
	return Action{
		Name:    "stream-ubuntu-image",
		Image:   imageTinkerbellImage2Disk,
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
		Image:   imageTinkerbellCexecPinned,
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ formatPartition ( index .Hardware.Disks 0 ) (.partition_number | int) }}",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": defaultInterpreter,
			"CMD_LINE":            fmt.Sprintf("growpart %s %s && resize2fs '{{ formatPartition ( index .Hardware.Disks 0 ) (.partition_number | int) }}'", destDisk, PartitionNumber),
		},
	}
}

func createNetworkConfigAction() Action {
	netplanConfig := `
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
		Image:   imageTinkerbellWriteFile,
		Timeout: 90,
		Environment: map[string]string{
			"DEST_DISK": "{{ formatPartition ( index .Hardware.Disks 0 ) (.partition_number | int) }}",
			"FS_TYPE":   fsType,
			"DEST_PATH": "/etc/netplan/config.yaml",
			"CONTENTS":  netplanConfig,
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
		Image:   imageTinkerbellActionsCexec,
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ formatPartition ( index .Hardware.Disks 0 ) (.partition_number | int) }}",
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
		Image:   imageTinkerbellCexecResolved,
		Timeout: 90,
		Environment: map[string]string{
			"BLOCK_DEVICE":        "{{ formatPartition ( index .Hardware.Disks 0 ) (.partition_number | int) }}",
			"FS_TYPE":             fsType,
			"CHROOT":              "y",
			"DEFAULT_INTERPRETER": "/bin/sh -c",
			"CMD_LINE":            fmt.Sprintf("cat /tmp/%s-bootstrap-config | base64 -d > '/var/lib/cloud/seed/nocloud/user-data'", hardwareName),
		},
	}
}

func createRebootAction() Action {
	return Action{
		Name:    "reboot-action",
		Image:   imageWaitDaemon,
		Pid:     "host",
		Timeout: 90,
		Command: []string{"reboot"},
		Environment: map[string]string{
			"IMAGE":        imageAlpine,
			"WAIT_SECONDS": "10",
		},
		Volumes: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
		},
	}
}
