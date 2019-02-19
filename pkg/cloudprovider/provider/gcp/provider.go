//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

// Environment variables for the configuration.
const (
	envGoogleClientID   = "GOOGLE_CLIENT_ID"
	envGoogleProjectID  = "GOOGLE_PROJECT_ID"
	envGoogleProjectID  = "GOOGLE_ZONE"
	envGoogleEmail      = "GOOGLE_EMAIL"
	envGooglePrivateKey = "GOOGLE_PRIVATE_KEY"
)

// Supported operating systems.
const (
	osUbuntu = "ubuntu-18.04"
	osCentOS = "centos-7"
)

// Terminal error messages.
const (
	errMachineSpec       = "Failed to parse MachineSpec: %v"
	errOperatingSystem   = "Invalid or not supported operating system specified %q: %v"
	errConnect           = "Failed to connect: %v"
	errInvalidClientID   = "Client ID is missing"
	errInvalidProjectID  = "Project ID is missing"
	errInvalidZone       = "Zone is missing"
	errInvalidEmail      = "Email is missing"
	errInvalidPrivateKey = "Private key is missing"
	errRetrieveInstance  = "Failed to retrieve instance: %v"
	errInsertInstance    = "Failed to insert instance: %v"
)

// nyiErr is a temporary error used during implementation. Has to be removed.
var nyiErr = fmt.Errorf("not yet implemented")

//-----
// Cloud Provider Specification
//-----

// cloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type cloudProviderSpec struct {
	ClientID   providerconfig.ConfigVarString `json:"clientID"`
	ProjectID  providerconfig.ConfigVarString `json:"projectID"`
	Zone       providerconfig.ConfigVarString `json:"zone"`
	Email      providerconfig.ConfigVarString `json:"email"`
	PrivateKey providerconfig.ConfigVarString `json:"privateKey"`
}

//-----
// Configuration
//-----

// config contains the configuration of the Provider.
type config struct {
	clientID       string
	projectID      string
	zone           string
	email          string
	privateKey     []byte
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
		providerConfig: providerConfig,
	}
	cfg.clientID, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ClientID, envGoogleClientID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve client ID: %v", err)
	}
	cfg.projectID, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.ProjectID, envGoogleProjectID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve project ID: %v", err)
	}
	cfg.zone, err = resolver.GetConfigVarStringValueOrEnv(cpSpec.Zone, envGoogleZone)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve zone: %v", err)
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
	return cfg, nil
}

//-----
// Provider
//-----

// Compile time verification of Provider implementing cloud.Provider.
var _ cloud.Provider = New(nil)

// Provider implements the cloud.Provider interface for the Google Cloud Platform.
type Provider struct {
	resolver *providerconfig.ConfigVarResolver
}

// New creates a cloud provider instance for the Google Cloud Platform.
func New(configVarResolver *providerconfig.ConfigVarResolver) *Provider {
	return &Provider{
		resolver: configVarResolver,
	}
}

// AddDefaults implements the cloud.Provider interface. It reads the MachineSpec and
// applies defaults for provider specific fields
func (p *Provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	// So far nothing to add.
	return spec, nil
}

// Validate implements the cloud.Provider interface. It validates the given
// machine's specification.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	// Read configuration.
	cfg, err := newConfig(p.resolver, spec)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Check configured values.
	if cfg.clientID == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidClientID)
	}
	if cfg.projectID == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidProjectID)
	}
	if cfg.zone == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidZone)
	}
	if cfg.email == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidEmail)
	}
	if len(cfg.privateKey) == 0 {
		return newError(common.InvalidConfigurationMachineError, errInvalidPrivateKey)
	}
	_, err = nameForOS(cfg.providerConfig.OperatingSystem)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errOperatingSystem, cfg.providerConfig.OperatingSystem, err)
	}
	return nil
}

// Get implements the cloud.Provider interface. It gets a node that is associated
// with the given machine.
func (p *Provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Retrieve instance.
	inst, err := svc.Instances.Get(cfg.projectID, cfg.zone, string(machine.UID)).Do()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errRetrieveInstance, err)
	}
	return &instance{inst}, nil
}

// GetCloudConfig implements the cloud.Provider interface. It returns the cloud provider specific
// cloud-config, which gets consumed by the kubelet.
func (p *Provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nyiErr
}

// Create implements the cloud.Provider interface. It creates a cloud instance according
// to the given machine.
func (p *Provider) Create(
	machine *clusterv1alpha1.Machine,
	data *MachineCreateDeleteData,
	userdata string,
) (instance.Instance, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	_, err = nameForOS(cfg.providerConfig.OperatingSystem)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errOperatingSystem, cfg.providerConfig.OperatingSystem, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Create GCP instance spec and insert it.
	inst := &compute.Instance{}
	op, err := svc.Instances.Insert(cfg.projectID, cfg.zone, inst).Do()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errInsertInstance, err)
	}
	err = svc.waitOperation(projectID, operation, timeoutNormal)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errInsertInstance, err)
	}
	// Retrieve it to get a full qualified instance.
	return p.Get(machine)
}

//-----
// Private helpers
//-----

// newError creates a terminal error matching to the provider interface.
func newError(reason common.MachineStatusError, msg string, args ...interface{}) error {
	return errors.TerminalError{
		Reason:  reason,
		Message: fmt.Sprintf(msg, args...),
	}
}

// nameForOS retrieves the operating system out of the provider configuration.
// Has to be supported.
func nameForOS(os providerconfig.OperatingSystem) (string, error) {
	switch os {
	case providerconfig.OperatingSystemUbuntu:
		return osUbuntu, nil
	case providerconfig.OperatingSystemCentOS:
		return osCentOS, nil
	}
	return "", providerconfig.ErrOSNotSupported
}
