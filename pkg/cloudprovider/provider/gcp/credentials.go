//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

//-----
// Credentials
//-----

// Credentials manages the OAuth2 credentials for the access to the GCP.
type Credentials struct {
	// ClientID is the OAuth ID for the GCP account.
	ClientID string

	// ProjectID is the identifier of the GCP project.
	ProjectID string

	// Email is the email address of the GCP account owner.
	Email string

	// PrivateKey is the private key matching to the public key
	// of the GCP account.
	PrivateKey []byte
}
