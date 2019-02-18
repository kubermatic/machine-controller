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

	compute "google.golang.org/api/compute/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

// Environment variables for the credentials.
const (
	envGoogleClientID   = "GOOGLE_CLIENT_ID"
	envGoogleProjectID  = "GOOGLE_PROJECT_ID"
	envGoogleEmail      = "GOOGLE_EMAIL"
	envGooglePrivateKey = "GOOGLE_PRIVATE_KEY"
)

// nyiErr is a temporary error used during implementation. Has to be removed.
var nyiErr = fmt.Errorf("not yet implemented")

//-----
// Cloud Provider Specification
//-----

// cloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
// TODO: Check how to handle private key, it's a []byte.
type cloudProviderSpec struct {
	ClientID   providerconfig.ConfigVarString `json:"clientID"`
	ProjectID  providerconfig.ConfigVarString `json:"projectID"`
	Email      providerconfig.ConfigVarString `json:"email"`
	PrivateKey providerconfig.ConfigVarString `json:"privateKey"`
}

//-----
// Config
//-----

// Config contains the configuration of the Provider.
type Config struct {
	clientID       string
	projectID      string
	email          string
	privateKey     []byte
	providerConfig *providerconfig.Config
}

// newConfig create a Provider configuration out of the passed resolver and spec.
func newConfig(r *providerconfig.ConfigVarResolver, s v1alpha1.ProviderSpec) (*Config, error) {
	// Retrieve provider configuration from machine specification.
	if s.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfig.Config{}
	err := json.Unmarshal(s.Value.Raw, &providerConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal machine.spec.providerconfig.value: %v", err)
	}
	// Retrieve cloud provider specification from cloud provider specification.
	spec := cloudProviderSpec{}
	err = json.Unmarshal(providerConfig.CloudProviderSpec.Raw, &spec)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal cloud provider specification: %v", err)
	}
	// Setup configuration.
	cfg := &Config{
		providerConfig: providerConfig,
	}
	cfg.clientID, err = r.GetConfigVarStringValueOrEnv(spec.ClientID, envGoogleClientID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve client ID: %v", err)
	}
	cfg.projectID, err = r.GetConfigVarStringValueOrEnv(spec.ProjectID, envGoogleProjectID)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve project ID: %v", err)
	}
	cfg.email, err = r.GetConfigVarStringValueOrEnv(spec.Email, envGoogleEmail)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve email: %v", err)
	}
	cfg.privateKey, err = r.GetConfigVarStringValueOrEnv(spec.PrivateKey, envGooglePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve private key: %v", err)
	}
	return cfg, nil
}

//-----
// Provider
//-----

// Compile time verification of Provider implementing cloud.Provider.
var _ cloud.Provider = New(nil)

// Provider implements the cloud.Provider interface for the Google Cloud Platform.
type Provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
	client            *metadata.Client
}

// New creates a cloud provider instance for the Google Cloud Platform.
func New(configVarResolver *providerconfig.ConfigVarResolver) *Provider {
	return &Provider{
		configVarResolver: configVarResolver,
	}
}

// AddDefaults implements the cloud.Provider interface.
func (p *Provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return nil, nyiErr
}

// Validate implements the cloud.Provider interface.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	return nyiErr
}

//-----
// Private helpers
//-----

// userAgentTransport sets the User-Agent header before calling base.
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

// connectComputeService establishes a service connection to the Compute Engine.
func connectComputeService(cfg *Config) (*compute.Service, error) {
	oauthClient := &http.Client{
		Transport: userAgentTransport{
			userAgent: "kubermatic-machine-controller",
			base:      http.DefaultTransport,
		},
	}
	svc, err := compute.New(oauthClient)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud Platform: %v", err)
	}
	return svc, nil
}
