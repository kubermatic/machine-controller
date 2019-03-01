//
// Google Cloud Provider for the Machine Controller
//

package gce

//-----
// Imports
//-----

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

// Environment variables for the configuration of the Google Cloud project access.
const (
	envGoogleServiceAccount = "GOOGLE_SERVICE_ACCOUNT"
)

// imageProjects maps the OS to the Google Cloud image projects
var imageProjects = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-cloud",
	providerconfig.OperatingSystemUbuntu: "ubuntu-os-cloud",
	providerconfig.OperatingSystemCentOS: "centos-cloud",
}

// imageFamilies maps the OS to the Google Cloud image projects
var imageFamilies = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-stable",
	providerconfig.OperatingSystemUbuntu: "ubuntu-1804-lts",
	providerconfig.OperatingSystemCentOS: "centos-7",
}

// diskTypes are the disk types of the Google Cloud. Map is used for
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
	ServiceAccount providerconfig.ConfigVarString `json:"serviceAccount"`
	Zone           providerconfig.ConfigVarString `json:"zone"`
	MachineType    providerconfig.ConfigVarString `json:"machineType"`
	DiskSize       int64                          `json:"diskSize"`
	DiskType       providerconfig.ConfigVarString `json:"diskType"`
	Labels         map[string]string              `json:"labels"`
}

//-----
// Configuration
//-----

// config contains the configuration of the Provider.
type config struct {
	serviceAccount string
	projectID      string
	zone           string
	machineType    string
	diskSize       int64
	diskType       string
	labels         map[string]string
	jwtConfig      *jwt.Config
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
	cfg.serviceAccount, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ServiceAccount, envGoogleServiceAccount)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve service account: %v", err)
	}
	err = cfg.postprocessServiceAccount()
	if err != nil {
		return nil, fmt.Errorf("cannot prepare JWT: %v", err)
	}
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
	cfg.labels = cpSpec.Labels
	return cfg, nil
}

// postprocessServiceAccount processes the service account and creates a JWT configuration
// out of it.
func (cfg *config) postprocessServiceAccount() error {
	if strings.Contains(cfg.serviceAccount, "\\n") {
		cfg.serviceAccount = strings.Replace(cfg.serviceAccount, "\\n", "\n", -1)
	}
	sam := map[string]string{}
	err := json.Unmarshal([]byte(cfg.serviceAccount), &sam)
	if err != nil {
		return fmt.Errorf("failed unmarshalling service account: %v", err)
	}
	cfg.projectID = sam["project_id"]
	sa, err := json.Marshal(sam)
	if err != nil {
		return fmt.Errorf("failed marshalling service account: %v", err)
	}
	cfg.jwtConfig, err = google.JWTConfigFromJSON(sa, compute.ComputeScope)
	if err != nil {
		return fmt.Errorf("failed preparing JWT: %v", err)
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
