//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

// Environment variables for the configuration of the Google Cloud project access.
const (
	envGoogleServiceAccount = "GOOGLE_SERVICE_ACCOUNT"
)

// imageProjects maps the OS to the Google Cloud image projects
var imageProjects = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-cloud",
	providerconfig.OperatingSystemUbuntu: "ubuntu-os-cloud",
}

// imageFamilies maps the OS to the Google Cloud image projects
var imageFamilies = map[providerconfig.OperatingSystem]string{
	providerconfig.OperatingSystemCoreos: "coreos-stable",
	providerconfig.OperatingSystemUbuntu: "ubuntu-1804-lts",
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

// cloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type cloudProviderSpec struct {
	ServiceAccount providerconfig.ConfigVarString `json:"serviceAccount"`
	Zone           providerconfig.ConfigVarString `json:"zone"`
	MachineType    providerconfig.ConfigVarString `json:"machineType"`
	DiskSize       int64                          `json:"diskSize"`
	DiskType       providerconfig.ConfigVarString `json:"diskType"`
	Network        providerconfig.ConfigVarString `json:"network"`
	Subnetwork     providerconfig.ConfigVarString `json:"subnetwork"`
	Labels         map[string]string              `json:"labels"`
}

// newCloudProviderSpec creates a cloud provider specification out of the
// given ProviderSpec.
func newCloudProviderSpec(spec v1alpha1.ProviderSpec) (*cloudProviderSpec, *providerconfig.Config, error) {
	// Retrieve provider configuration from machine specification.
	if spec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, &providerConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal machine.spec.providerconfig.value: %v", err)
	}
	// Retrieve cloud provider specification from cloud provider specification.
	cpSpec := &cloudProviderSpec{}
	err = json.Unmarshal(providerConfig.CloudProviderSpec.Raw, cpSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal cloud provider specification: %v", err)
	}
	return cpSpec, &providerConfig, nil
}

// updateProviderSpec updates the given provider spec with changed
// configuration values.
func (cpSpec *cloudProviderSpec) updateProviderSpec(spec v1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if spec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, &providerConfig)
	if err != nil {
		return nil, err
	}
	rawCPSpec, err := json.Marshal(cpSpec)
	if err != nil {
		return nil, err
	}
	providerConfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCPSpec}
	rawProviderConfig, err := json.Marshal(providerConfig)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: rawProviderConfig}, nil
}

// config contains the configuration of the Provider.
type config struct {
	serviceAccount string
	projectID      string
	zone           string
	machineType    string
	diskSize       int64
	diskType       string
	network        string
	subnetwork     string
	labels         map[string]string
	jwtConfig      *jwt.Config
	providerConfig *providerconfig.Config
}

// newConfig creates a Provider configuration out of the passed resolver and spec.
func newConfig(resolver *providerconfig.ConfigVarResolver, spec v1alpha1.ProviderSpec) (*config, error) {
	// Create cloud provider spec.
	cpSpec, providerConfig, err := newCloudProviderSpec(spec)
	if err != nil {
		return nil, err
	}

	// Setup configuration.
	cfg := &config{
		providerConfig: providerConfig,
		labels:         cpSpec.Labels,
		diskSize:       cpSpec.DiskSize,
		network:        cpSpec.Network.Value,
		subnetwork:     cpSpec.Subnetwork.Value,
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

	cfg.diskType, err = resolver.GetConfigVarStringValue(cpSpec.DiskType)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve disk type: %v", err)
	}

	return cfg, nil
}

// postprocessServiceAccount processes the service account and creates a JWT configuration
// out of it.
func (cfg *config) postprocessServiceAccount() error {
	sa, err := base64.StdEncoding.DecodeString(cfg.serviceAccount)
	if err != nil {
		return fmt.Errorf("failed to decode base64 service account: %v", err)
	}
	sam := map[string]string{}
	err = json.Unmarshal(sa, &sam)
	if err != nil {
		return fmt.Errorf("failed unmarshalling service account: %v", err)
	}
	cfg.projectID = sam["project_id"]
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
