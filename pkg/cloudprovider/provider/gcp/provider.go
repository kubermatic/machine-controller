//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"errors"
	"fmt"
	"net/http"

	"cloud.google.com/go/compute/metadata"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Config
//-----

// Config contains the configuration of the Provider.
type Config struct {
	providerConfig *providerconfig.Config
}

// newConfig create a Provider configuration out of the passed resolver and spec.
func newConfig(r *providerconfig.ConfigVarResolver, s v1alpha1.ProviderSpec) (*Config, error) {
	return nil, errors.New("not yet implemented")
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
	return nil, errors.New("not yet implemented")
}

// Validate implements the cloud.Provider interface.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	return errors.New("not yet implemented")
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
