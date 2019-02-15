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

	"cloud.google.com/go/compute/metadata"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

const (
	// envGCPProjectID is the environment variable for the project ID.
	envGCPProjectID = "GOOGLE_PROJECT_ID"
)

// nyiErr is a temporary error used during implementation. Has to be removed.
var nyiErr = fmt.Errorf("not yet implemented")

//-----
// Config
//-----

// cloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type cloudProviderSpec struct {
	ProjectID providerconfig.ConfigVarString `json:"projectID"`
}

// Config contains the configuration of the Provider.
type Config struct {
	projectID      string
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
	c.projectID, err = r.GetConfigVarStringValueOrEnv(spec.ProjectID, envGCPProjectID)
	if err != nil {
		return nil, fmt.Errorf(`failed to get the value of "projectID" field: %v`, err)
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
// Private helpers.
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

// connectGCP returns a client for communication with the GCP.
func connectGCP() (*metadata.Client, error) {
	c := metadata.NewClient(&http.Client{
		Transport: userAgentTransport{
			userAgent: "kubermatic-machine-controller",
			base:      http.DefaultTransport,
		},
	})
	p, err := c.ProjectID()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud Platform: %v", err)
	}
	return c, nil
}
