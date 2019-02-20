//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

// Environment variables for the configuration of the GCP project access.
const (
	envGoogleClientID   = "GOOGLE_CLIENT_ID"
	envGoogleProjectID  = "GOOGLE_PROJECT_ID"
	envGoogleEmail      = "GOOGLE_EMAIL"
	envGooglePrivateKey = "GOOGLE_PRIVATE_KEY"
)

// imageProjects maps the OS to the GCP image projects
var imageProjects = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-cloud",
	providerconfig.OperatingSystemUbuntu: "ubuntu-os-cloud",
	providerconfig.OperatingSystemCentOS: "centos-cloud",
}

// imageFamilies maps the OS to the GCP image projects
var imageFamilies = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-stable",
	providerconfig.OperatingSystemUbuntu: "ubuntu-1804-lts",
	providerconfig.OperatingSystemCentOS: "centos-7",
}

// diskTypes are the disk types of the GCP. Map is used for
// validation.
var diskTypes = map[string]bool{
	"pd-standard": true,
	"pd-ssd":      true,
}

//-----
// Cloud Provider Specification
//-----

// cloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type cloudProviderSpec struct {
	ClientID    providerconfig.ConfigVarString `json:"clientID"`
	ProjectID   providerconfig.ConfigVarString `json:"projectID"`
	Email       providerconfig.ConfigVarString `json:"email"`
	PrivateKey  providerconfig.ConfigVarString `json:"privateKey"`
	Zone        providerconfig.ConfigVarString `json:"zone"`
	MachineType providerconfig.ConfigVarString `json:"machineType"`
	DiskSize    int64                          `json:"diskSize"`
	DiskType    providerconfig.ConfigVarString `json:"diskType"`
}

//-----
// Configuration
//-----

// config contains the configuration of the Provider.
type config struct {
	clientID       string
	projectID      string
	email          string
	privateKey     []byte
	zone           string
	machineType    string
	diskSize       int64
	diskType       string
	providerConfig *providerconfig.Config
}

// newConfig create a Provider configuration out of the passed resolver and spec.
func newConfig(resolver *providerconfig.ConfigVarResolver, spec v1alpha1.ProviderSpec) (*config, error) {
	// Retrieve provider configuration from machine specification.
	if spec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, &providerConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal machine.spec.providerconfig.value: %v", err)
	}
	// Retrieve cloud provider specification from cloud provider specification.
	cpSpec := cloudProviderSpec{}
	err = json.Unmarshal(providerConfig.CloudProviderSpec.Raw, &cpSpec)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal cloud provider specification: %v", err)
	}
	// Setup configuration.
	cfg := &config{
		providerConfig: &providerConfig,
	}
	cfg.clientID, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ClientID, envGoogleClientID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve client ID: %v", err)
	}
	cfg.projectID, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ProjectID, envGoogleProjectID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve project ID: %v", err)
	}
	cfg.email, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.Email, envGoogleEmail)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve email: %v", err)
	}
	var pks string
	pks, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.PrivateKey, envGooglePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve private key: %v", err)
	}
	cfg.privateKey = []byte(pks)
	cfg.zone, err = resolver.GetConfigVarStringValue(cpSpec.Zone)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve zone: %v", err)
	}
	cfg.machineType, err = resolver.GetConfigVarStringValue(cpSpec.MachineType)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve machine type: %v", err)
	}
	cfg.diskSize = cpSpec.DiskSize
	cfg.diskType, err = resolver.GetConfigVarStringValue(cpSpec.DiskType)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve disk type: %v", err)
	}
	return cfg, nil
}

// machineTypeDescriptor creates the descriptor out of zone and machine type
// for the machine type of an instance.
func (cfg *config) machineTypeDescriptor() string {
	return fmt.Sprintf("zones/%s/machineTypes/%s", cfg.zone, cfg.machineType)
}

// sourceImageDescriptor creates the descriptor out of project and family
// for the source image of an instance boot disk.
func (cfg *config) sourceImageDescriptor() (string, error) {
	project, ok := imageProjects[cfg.providerConfig.OperatingSystem]
	if !ok {
		return "", providerconfig.ErrOSNotSupported
	}
	family, ok := imageFamilies[cfg.providerConfig.OperatingSystem]
	if !ok {
		return "", providerconfig.ErrOSNotSupported
	}
	return fmt.Sprintf("projects/%s/global/images/family/%s", project, family), nil
}
