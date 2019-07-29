package cherrygo

import (
	"log"
	"strings"
)

const baseIPSPath = "/v1/projects"
const endIPSPath = "ips"

// GetIPS interface metodas isgauti team'sus
type GetIPS interface {
	List(projectID string) ([]IPAddresses, *Response, error)
}

// IPSClient paveldi client
type IPSClient struct {
	client *Client
}

// List func lists teams
func (i *IPSClient) List(projectID string) ([]IPAddresses, *Response, error) {
	//root := new(teamRoot)

	ipsPath := strings.Join([]string{baseIPSPath, projectID, endIPSPath}, "/")

	var trans []IPAddresses
	//resp := t.client.Bumba()
	//log.Println("\nFROM LIST1: ", root.Teams)
	resp, err := i.client.MakeRequest("GET", ipsPath, nil, &trans)
	if err != nil {
		log.Fatalf("Error while making request: %v", err)
	}

	return trans, resp, err
}
