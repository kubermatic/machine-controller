package cherrygo

import (
	"fmt"
	"strings"
)

// GetSSHKey interface
type GetSSHKey interface {
	List(sshKeyID string) (SSHKey, *Response, error)
	Create(request *CreateSSHKey) (SSHKeys, *Response, error)
	Delete(request *DeleteSSHKey) (SSHKeys, *Response, error)
	Update(sshKeyID string, request *UpdateSSHKey) (SSHKeys, *Response, error)
}

// SSHKey fields for return values after creation
type SSHKey struct {
	ID          int    `json:"id,omitempty"`
	Label       string `json:"label,omitempty"`
	Key         string `json:"key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Updated     string `json:"updated,omitempty"`
	Created     string `json:"created,omitempty"`
	Href        string `json:"href,omitempty"`
}

// CreateSSHKey fields for adding new key with label and raw key
type CreateSSHKey struct {
	Label string `json:"label,omitempty"`
	Key   string `json:"key,omitempty"`
}

// DeleteSSHKey fields for key delition by its ID
type DeleteSSHKey struct {
	ID string `json:"id,omitempty"`
}

// UpdateSSHKey fields for label or key update
type UpdateSSHKey struct {
	Label string `json:"label,omitempty"`
	Key   string `json:"key,omitempty"`
}

// SSHKeyClient paveldi client
type SSHKeyClient struct {
	client *Client
}

// List func lists all available ssh keys
func (s *SSHKeyClient) List(sshKeyID string) (SSHKey, *Response, error) {
	//root := new(teamRoot)

	var trans SSHKey

	sshKeyPath := strings.Join([]string{baseSSHPath, sshKeyID}, "/")

	resp, err := s.client.MakeRequest("GET", sshKeyPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}

// Create adds new SSH key
func (s *SSHKeyClient) Create(request *CreateSSHKey) (SSHKeys, *Response, error) {

	var trans SSHKeys

	resp, err := s.client.MakeRequest("POST", baseSSHPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}

// Delete removes desired SSH key by its ID
func (s *SSHKeyClient) Delete(request *DeleteSSHKey) (SSHKeys, *Response, error) {

	var trans SSHKeys

	sshKeyPath := strings.Join([]string{baseSSHPath, request.ID}, "/")

	resp, err := s.client.MakeRequest("DELETE", sshKeyPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}

// Update function updates keys Label or key itself
func (s *SSHKeyClient) Update(sshKeyID string, request *UpdateSSHKey) (SSHKeys, *Response, error) {

	var trans SSHKeys

	sshKeyPath := strings.Join([]string{baseSSHPath, sshKeyID}, "/")

	resp, err := s.client.MakeRequest("PUT", sshKeyPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}
