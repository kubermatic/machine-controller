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

import "github.com/tinkerbell/tink/workflow"

type workflowConfig struct {
	WorkerMACAddress  string
	TinkServerAddress string
	CloudInit         string
}

func createTemplate(config *workflowConfig) *workflow.Workflow {
	return &workflow.Workflow{
		Version:       "0.1",
		Name:          "ubuntu_provisioning",
		GlobalTimeout: 6000,
		Tasks: []workflow.Task{
			{
				Name:       "os-installation",
				WorkerAddr: config.WorkerMACAddress,
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
							"MIRROR_HOST": config.TinkServerAddress,
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
							"MIRROR_HOST": config.TinkServerAddress,
						},
						Volumes: nil,
					},
					{
						Name:    "install-grub",
						Image:   "install-grub:v1",
						Timeout: 600,
						Environment: map[string]string{
							"MIRROR_HOST": config.TinkServerAddress,
						},
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
					{
						Name:    "cloud-init",
						Image:   "cloud-init:v3",
						Timeout: 600,
						Environment: map[string]string{
							"MIRROR_HOST":       config.TinkServerAddress,
							"CUSTOM_CLOUD_INIT": config.CloudInit,
						},
						Volumes: []string{
							"/statedir:/statedir",
						},
					},
				},
			},
		},
	}
}
