package cherrygo

import "fmt"

const baseSSHPath = "/v1/ssh-keys"

// GetSSHKeys interface metodas isgauti team'sus
type GetSSHKeys interface {
	List() ([]SSHKeys, *Response, error)
}

// SSHKeys fields for return values after creation
type SSHKeys struct {
	ID          int    `json:"id,omitempty"`
	Label       string `json:"label,omitempty"`
	Key         string `json:"key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Updated     string `json:"updated,omitempty"`
	Created     string `json:"created,omitempty"`
	Href        string `json:"href,omitempty"`
}

// SSHKeysClient paveldi client
type SSHKeysClient struct {
	client *Client
}

// List func lists all available ssh keys
func (s *SSHKeysClient) List() ([]SSHKeys, *Response, error) {
	//root := new(teamRoot)

	var trans []SSHKeys
	//resp := t.client.Bumba()
	//log.Println("\nFROM LIST1: ", root.Teams)
	resp, err := s.client.MakeRequest("GET", baseSSHPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}
