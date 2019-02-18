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

// Possible instance statuses.
const (
	statusInstanceProvisioning = "PROVISIONING"
	statusInstanceRunning      = "RUNNING"
	statusInstanceStaging      = "STAGING"
	statusInstanceStopped      = "STOPPED"
	statusInstanceStopping     = "STOPPING"
	statusInstanceSuspended    = "SUSPENDED"
	statusInstanceSuspending   = "SUSPENDING"
	statusInstanceTerminated   = "TERMINATED"
)

// Supported operating systems.
const (
	osUbuntu = "ubuntu-18.04"
	osCentOS = "centos-7"
)

// connectionType describes the kind of GCP connection.
const connectionType = "service_account"

// driverScopes addresses the parts of the GCP API the provider addresses.
var (
	driverScopes = []string{
		"https://www.googleapis.com/auth/compute",
	}
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

// AddDefaults implements the cloud.Provider interface.
func (p *Provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	// So far nothing to add.
	return spec, nil
}

// Validate implements the cloud.Provider interface.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	// Read configuration.
	cfg, err := newConfig(p.resolver, spec)
	if err != nil {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec: %v", err),
		}
	}
	// Check configured values.
	if cfg.clientID == "" {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Client ID is missing"),
		}
	}
	if cfg.projectID == "" {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Project ID is missing"),
		}
	}
	if cfg.zone == "" {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Zone is missing"),
		}
	}
	if cfg.email == "" {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Email is missing"),
		}
	}
	if len(cfg.privateKey) == 0 {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Private key is missing"),
		}
	}
	_, err = nameForOS(cfg.providerConfig.OperatingSystem)
	if err != nil {
		return errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Invalid or not supported operating system specified %q: %v", cfg.providerConfig.OperatingSystem, err),
		}
	}
	return nil
}

// Get implements the cloud.Provider interface.
func (p *Provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec: %v", err),
		}
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return nil, errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to connect: %v", err),
		}
	}
	// Retrieve instance.
	inst, err := svc.Instances.Get(cfg.projectID, cfg.zone, string(machine.UID)).Do()
	if err != nil {
		return nil, errors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to retrieve instance : %v", err),
		}
	}
	return &gcpInstance{
		inst: inst,
	}, nil
}

//-----
// Instance
//-----

// gcpInstance implements instance.Instance for the GCP instance.
type gcpInstance struct {
	inst *compute.Instance
}

// Name implements instance.Instance.
func (gcp *gcpInstance) Name() string {
	return gcp.inst.Name
}

// ID implements instance.Instance.
func (gcp *gcpInstance) ID() string {
	return strconv.FormatUint(gcp.inst.Id, 10)
}

// Addresses implements instance.Instance.
func (gcp *gcpInstance) Addresses() []string {
	var addrs []string
	for _, ifc := range gcp.inst.NetworkInterfaces {
		addrs = append(addrs, ifc.NetworkIP)
	}
	return addrs
}

// Status implements instance.Instance.
// TODO Check status mapping for staging, delet(ed|ing), suspend(ed|ing).
func (gcp *gcpInstance) Status() instance.Status {
	switch gcp.inst.Status {
	case statusInstanceProvisioning:
		return instance.StatusCreating
	case statusInstanceRunning:
		return instance.StatusRunning
	case statusInstanceStaging:
		return instance.StatusCreating
	case statusInstanceStopped:
		return instance.StatusDeleted
	case statusInstanceStopping:
		return instance.StatusDeleting
	case statusInstanceSuspended:
		return instance.StatusDeleted
	case statusInstanceSuspending:
		return instance.StatusDeleting
	case statusInstanceTerminated:
		return instance.StatusDeleted
	}
	// Must not happen.
	return instance.StatusUnknown
}

//-----
// Private helpers
//-----

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

// connectComputeService establishes a service connection to the Compute Engine.
func connectComputeService(cfg *Config) (*compute.Service, error) {
	jsonMap := map[string]string{
		"type":         connectionType,
		"client_id":    cfg.clientID,
		"client_email": cfg.email,
		"private_key":  string(cfg.privateKey),
	}
	jsonBytes, err := json.Marshal(jsonMap)
	if err != nil {
		return nil, fmt.Errorf("cannot create credentials: %v", err)
	}
	cfg, err := google.JWTConfigFromJSON(jsonBytes, driverScopes...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := cfg.Client(oauth2.NoContext)
	svc, err := compute.New(client)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud Platform: %v", err)
	}
	return svc, nil
}
