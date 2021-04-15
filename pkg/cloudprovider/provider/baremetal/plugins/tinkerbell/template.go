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

package tinkerbell

import (
	"github.com/tinkerbell/tink/workflow"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
)

const (
	provisioningTemplate = "ubuntu_os_provisioning"
)

func createTemplate(worker, tinkServerAddress, imageRepoAddress string, cfg *plugins.CloudConfigSettings) *workflow.Workflow {
	return &workflow.Workflow{
		Version:       "0.1",
		Name:          provisioningTemplate,
		ID:            "",
		GlobalTimeout: 6000,
		Tasks: []workflow.Task{
			{
				Name:       "os-installation",
				WorkerAddr: worker,
				Volumes: []string{
					"/dev:/dev",
					"/dev/console:/dev/console",
					"/lib/firmware:/lib/firmware:ro",
				},
				Actions: []workflow.Action{
					{
						Name:    "disk-wipe",
						Image:   "disk-wipe:v1",
						Timeout: 90,
					},
					{
						Name:    "disk-partition",
						Image:   "disk-partition:v1",
						Timeout: 180,
						Environment: map[string]string{
							"MIRROR_HOST": tinkServerAddress,
						},
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
					{
						Name:    "install-root-fs",
						Image:   "install-root-fs:v1",
						Timeout: 600,
						Environment: map[string]string{
							"MIRROR_HOST": imageRepoAddress,
						},
						Volumes: nil,
					},
					{
						Name:    "install-grub",
						Image:   "install-grub:v1",
						Timeout: 600,
						Environment: map[string]string{
							"MIRROR_HOST": imageRepoAddress,
						},
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
					{
						Name:    "cloud-init",
						Image:   "cloud-init:v1",
						Timeout: 600,
						Environment: map[string]string{
							"MIRROR_HOST":                   imageRepoAddress,
							"CLOUD_INIT_TOKEN":              cfg.Token,
							"CLOUD_INIT_SETTINGS_NAMESPACE": cfg.Namespace,
							"SECRET_NAME":                   cfg.SecretName,
							"CLUSTER_HOST":                  cfg.ClusterHost,
						},
					},
				},
			},
		},
	}
}
