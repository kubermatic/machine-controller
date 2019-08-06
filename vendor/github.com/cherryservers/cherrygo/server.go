package cherrygo

import (
	"fmt"
	"strings"
)

const baseServerPath = "/v1/servers"
const serverActionPath = "actions"
const powerPath = "?fields=power"

// GetServer interface metodas isgauti team'sus
type GetServer interface {
	List(serverID string) (Server, *Response, error)
	PowerOff(serverID string) (Server, *Response, error)
	PowerOn(serverID string) (Server, *Response, error)
	Create(projectID string, request *CreateServer) (Server, *Response, error)
	Delete(request *DeleteServer) (Server, *Response, error)
	PowerState(serverID string) (PowerState, *Response, error)
	Reboot(serverID string) (Server, *Response, error)
}

// Server tai ka grazina api
type Server struct {
	ID               int              `json:"id,omitempty"`
	Name             string           `json:"name,omitempty"`
	Href             string           `json:"href,omitempty"`
	Hostname         string           `json:"hostname,omitempty"`
	Image            string           `json:"image,omitempty"`
	Region           Region           `json:"region,omitempty"`
	State            string           `json:"state,omitempty"`
	Plans            Plans            `json:"plan,omitempty"`
	AvailableRegions AvailableRegions `json:"availableregions,omitempty"`
	Pricing          Pricing          `json:"pricing,omitempty"`
	IPAddresses      []IPAddresses    `json:"ip_addresses,omitempty"`
	SSHKeys          []SSHKeys        `json:"ssh_keys,omitempty"`
}

// ServerClient paveldi client
type ServerClient struct {
	client *Client
}

// ServerAction fields for performed action on server
type ServerAction struct {
	Type string `json:"type,omitempty"`
}

// PowerState fields
type PowerState struct {
	Power string `json:"power,omitempty"`
}

// CreateServer fields for ordering new server
type CreateServer struct {
	ProjectID   string   `json:"project_id,omitempty"`
	PlanID      string   `json:"plan_id,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	Image       string   `json:"image,omitempty"`
	Region      string   `json:"region,omitempty"`
	SSHKeys     []string `json:"ssh_keys"`
	IPAddresses []string `json:"ip_addresses"`
	UserData    string   `json:"user_data"`
}

// DeleteServer field for removing server
type DeleteServer struct {
	ID string `json:"id,omitempty"`
}

// List func lists teams
func (s *ServerClient) List(serverID string) (Server, *Response, error) {
	//root := new(teamRoot)

	//serverIDString := strconv.Itoa(serverID)

	serverPath := strings.Join([]string{baseServerPath, serverID}, "/")

	var trans Server

	resp, err := s.client.MakeRequest("GET", serverPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}

// PowerState func
func (s *ServerClient) PowerState(serverID string) (PowerState, *Response, error) {

	//serverIDString := strconv.Itoa(serverID)

	serverPath := strings.Join([]string{baseServerPath, serverID + powerPath}, "/")

	var trans PowerState

	resp, err := s.client.MakeRequest("GET", serverPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}

// Create function orders new floating IP address
func (s *ServerClient) Create(projectID string, request *CreateServer) (Server, *Response, error) {

	var trans Server

	//projectIDString := strconv.Itoa(projectID)

	serverPath := strings.Join([]string{baseIPSPath, projectID, endServersPath}, "/")

	resp, err := s.client.MakeRequest("POST", serverPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err

}

// Delete removes desired SSH key by its ID
func (s *ServerClient) Delete(request *DeleteServer) (Server, *Response, error) {

	var trans Server

	serverPath := strings.Join([]string{baseServerPath, request.ID}, "/")

	resp, err := s.client.MakeRequest("DELETE", serverPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}

// PowerOn function turns server on
func (s *ServerClient) PowerOn(serverID string) (Server, *Response, error) {

	var trans Server

	powerOffRequest := ServerAction{
		Type: "power_on",
	}

	serverPath := strings.Join([]string{baseServerPath, serverID, serverActionPath}, "/")

	resp, err := s.client.MakeRequest("POST", serverPath, powerOffRequest, &trans)

	return trans, resp, err
}

// PowerOff function turns server off
func (s *ServerClient) PowerOff(serverID string) (Server, *Response, error) {

	var trans Server

	powerOffRequest := ServerAction{
		Type: "power_off",
	}

	serverPath := strings.Join([]string{baseServerPath, serverID, serverActionPath}, "/")

	resp, err := s.client.MakeRequest("POST", serverPath, powerOffRequest, &trans)

	return trans, resp, err
}

// Reboot function restarts desired server
func (s *ServerClient) Reboot(serverID string) (Server, *Response, error) {

	var trans Server

	rebootRequest := ServerAction{
		Type: "reboot",
	}

	serverPath := strings.Join([]string{baseServerPath, serverID, serverActionPath}, "/")

	resp, err := s.client.MakeRequest("POST", serverPath, rebootRequest, &trans)

	return trans, resp, err
}
