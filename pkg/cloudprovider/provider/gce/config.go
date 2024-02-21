/*
Copyright 2019 The Machine Controller Authors.

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

//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	gcetypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/gce/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

// Environment variables for the configuration of the Google Cloud project access.
const (
	envGoogleServiceAccount = "GOOGLE_SERVICE_ACCOUNT"
)

// imageProjects maps the OS to the Google Cloud image projects.
var imageProjects = map[providerconfigtypes.OperatingSystem]string{
	providerconfigtypes.OperatingSystemUbuntu:  "ubuntu-os-cloud",
	providerconfigtypes.OperatingSystemFlatcar: "kinvolk-public",
}

// imageFamilies maps the OS to the Google Cloud image projects.
var imageFamilies = map[providerconfigtypes.OperatingSystem]string{
	providerconfigtypes.OperatingSystemUbuntu:  "ubuntu-2204-lts",
	providerconfigtypes.OperatingSystemFlatcar: "flatcar-stable",
}

// diskTypes are the disk types of the Google Cloud. Map is used for
// validation.
var diskTypes = map[string]bool{
	"pd-standard": true,
	"pd-ssd":      true,
}

// Default values for disk type and size (in GB).
const (
	defaultDiskType = "pd-standard"
	defaultDiskSize = 25
)

// newCloudProviderSpec creates a cloud provider specification out of the
// given ProviderSpec.
func newCloudProviderSpec(provSpec v1alpha1.ProviderSpec) (*gcetypes.CloudProviderSpec, *providerconfigtypes.Config, error) {
	// Retrieve provider configuration from machine specification.
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal machine.spec.providerconfig.value: %w", err)
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	// Retrieve cloud provider specification from cloud provider specification.
	cpSpec, err := gcetypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal cloud provider specification: %w", err)
	}

	return cpSpec, pconfig, nil
}

// config contains the configuration of the Provider.
type config struct {
	serviceAccount               string
	projectID                    string
	zone                         string
	machineType                  string
	diskSize                     int64
	diskType                     string
	network                      string
	subnetwork                   string
	preemptible                  bool
	automaticRestart             *bool
	provisioningModel            *string
	labels                       map[string]string
	tags                         []string
	providerConfig               *providerconfigtypes.Config
	assignPublicIPAddress        bool
	multizone                    bool
	regional                     bool
	customImage                  string
	disableMachineServiceAccount bool
	enableNestedVirtualization   bool
	minCPUPlatform               string
	guestOSFeatures              []string
	clientConfig                 *clientConfig
}

type clientConfig struct {
	ClientEmail string
	TokenSource oauth2.TokenSource
}

// newConfig creates a Provider configuration out of the passed resolver and spec.
func newConfig(resolver *providerconfig.ConfigPointerVarResolver, spec v1alpha1.ProviderSpec) (*config, error) {
	// Create cloud provider spec.
	cpSpec, providerConfig, err := newCloudProviderSpec(spec)
	if err != nil {
		return nil, err
	}

	// Setup configuration.
	cfg := &config{
		providerConfig:  providerConfig,
		labels:          cpSpec.Labels,
		tags:            cpSpec.Tags,
		diskSize:        cpSpec.DiskSize,
		guestOSFeatures: cpSpec.GuestOSFeatures,
	}

	cfg.serviceAccount, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ServiceAccount, envGoogleServiceAccount)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve service account: %w", err)
	}

	cfg.projectID, err = resolver.GetConfigVarStringValue(&cpSpec.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve project id: %w", err)
	}

	err = cfg.postprocessServiceAccount()
	if err != nil {
		return nil, fmt.Errorf("cannot prepare JWT: %w", err)
	}

	cfg.zone, err = resolver.GetConfigVarStringValue(cpSpec.Zone)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve zone: %w", err)
	}

	cfg.machineType, err = resolver.GetConfigVarStringValue(cpSpec.MachineType)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve machine type: %w", err)
	}

	cfg.diskType, err = resolver.GetConfigVarStringValue(cpSpec.DiskType)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve disk type: %w", err)
	}

	cfg.network, err = resolver.GetConfigVarStringValue(cpSpec.Network)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve network: %w", err)
	}

	cfg.subnetwork, err = resolver.GetConfigVarStringValue(cpSpec.Subnetwork)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve subnetwork: %w", err)
	}

	cfg.preemptible, _, err = resolver.GetConfigVarBoolValue(cpSpec.Preemptible)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve preemptible: %w", err)
	}

	if cpSpec.AutomaticRestart != nil {
		automaticRestart, _, err := resolver.GetConfigVarBoolValue(cpSpec.AutomaticRestart)
		if err != nil {
			return nil, fmt.Errorf("cannot retrieve automaticRestart: %w", err)
		}
		cfg.automaticRestart = &automaticRestart

		if *cfg.automaticRestart && cfg.preemptible {
			return nil, fmt.Errorf("automatic restart option can only be enabled for standard instances. Preemptible instances cannot be automatically restarted")
		}
	}

	if cpSpec.ProvisioningModel != nil {
		provisioningModel, err := resolver.GetConfigVarStringValue(cpSpec.ProvisioningModel)
		if err != nil {
			return nil, fmt.Errorf("cannot retrieve provisioningModel: %w", err)
		}
		cfg.provisioningModel = &provisioningModel
	}

	// make it true by default
	cfg.assignPublicIPAddress = true

	if cpSpec.AssignPublicIPAddress != nil {
		cfg.assignPublicIPAddress, _, err = resolver.GetConfigVarBoolValue(cpSpec.AssignPublicIPAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve assignPublicIPAddress: %w", err)
		}
	}

	cfg.multizone, _, err = resolver.GetConfigVarBoolValue(cpSpec.MultiZone)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve multizone: %w", err)
	}

	cfg.regional, _, err = resolver.GetConfigVarBoolValue(cpSpec.Regional)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve regional: %w", err)
	}

	cfg.customImage, err = resolver.GetConfigVarStringValue(cpSpec.CustomImage)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve gce custom image: %w", err)
	}

	cfg.disableMachineServiceAccount, _, err = resolver.GetConfigVarBoolValue(cpSpec.DisableMachineServiceAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve disable machine service account: %w", err)
	}

	cfg.enableNestedVirtualization, _, err = resolver.GetConfigVarBoolValue(cpSpec.EnableNestedVirtualization)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve enable nested virtualization: %w", err)
	}

	cfg.minCPUPlatform, err = resolver.GetConfigVarStringValue(cpSpec.MinCPUPlatform)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve min cpu platform: %w", err)
	}

	return cfg, nil
}

// postprocessServiceAccount processes the service account and creates a JWT configuration
// out of it.
func (cfg *config) postprocessServiceAccount() error {
	sa := cfg.serviceAccount

	// safely decode the service account, in case we did not read the value
	// from a "known-safe" location (like the MachineDeployment), but from
	// an environment variable.
	decoded, err := base64.StdEncoding.DecodeString(cfg.serviceAccount)
	if err == nil {
		sa = string(decoded)
	}

	creds, err := googleoauth.CredentialsFromJSON(context.TODO(), []byte(sa), compute.ComputeScope)
	if err != nil {
		return fmt.Errorf("failed to parse credentials from google service account: %w", err)
	}

	if cfg.projectID == "" {
		cfg.projectID = creds.ProjectID
	}

	sam := map[string]string{}
	err = json.Unmarshal([]byte(sa), &sam)
	if err != nil {
		return fmt.Errorf("failed unmarshalling service account: %w", err)
	}

	// if the project id is not set in the machine deployment, we fallback to the project id that is embedded in the
	// google service account json object.
	cfg.clientConfig = &clientConfig{
		ClientEmail: sam["client_email"],
		TokenSource: creds.TokenSource,
	}

	return nil
}

// machineTypeDescriptor creates the descriptor out of zone and machine type
// for the machine type of an instance.
func (cfg *config) machineTypeDescriptor() string {
	return fmt.Sprintf("zones/%s/machineTypes/%s", cfg.zone, cfg.machineType)
}

// diskTypeDescriptor creates the descriptor out of zone and disk type
// for the disk type of an instance.
func (cfg *config) diskTypeDescriptor() string {
	return fmt.Sprintf("zones/%s/diskTypes/%s", cfg.zone, cfg.diskType)
}

// sourceImageDescriptor creates the descriptor out of project and family
// for the source image of an instance boot disk.
func (cfg *config) sourceImageDescriptor() (string, error) {
	if cfg.customImage != "" {
		// If a full image identifier is provided, use it
		if strings.HasPrefix("projects/", cfg.customImage) {
			return cfg.customImage, nil
		}

		// Otherwise, make sure to properly prefix the image identifier
		return fmt.Sprintf("global/images/%s", cfg.customImage), nil
	}
	project, ok := imageProjects[cfg.providerConfig.OperatingSystem]
	if !ok {
		return "", providerconfigtypes.ErrOSNotSupported
	}
	family, ok := imageFamilies[cfg.providerConfig.OperatingSystem]
	if !ok {
		return "", providerconfigtypes.ErrOSNotSupported
	}
	return fmt.Sprintf("projects/%s/global/images/family/%s", project, family), nil
}
