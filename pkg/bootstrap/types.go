/*
Copyright 2022 The Machine Controller Authors.

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

package bootstrap

type CloudConfigSecret string

const (
	ProvisioningCloudConfig CloudConfigSecret = "provisioning"
	BootstrapCloudConfig    CloudConfigSecret = "bootstrap"

	CloudConfigSecretNamePattern = "%s-%s-%s-config"

	// CloudInitSettingsNamespace is the namespace in which bootstrap secrets are created by an external mechanism
	CloudInitSettingsNamespace = "cloud-init-settings"
	// MachineDeploymentRevision is the revision for Machine Deployment.
	MachineDeploymentRevision = "k8c.io/machine-deployment-revision"
)
