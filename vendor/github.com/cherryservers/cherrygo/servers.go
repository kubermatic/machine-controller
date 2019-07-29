package cherrygo

import (
	"log"
	"strings"
)

const baseServersPath = "/v1/projects"
const endServersPath = "servers"

// GetServers interface metodas isgauti team'sus
type GetServers interface {
	List(projectID string) ([]Servers, *Response, error)
}

// Servers tai ka grazina api
type Servers struct {
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

// Region fields
type Region struct {
	ID         int    `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	RegionIso2 string `json:"region_iso_2,omitempty"`
	Href       string `json:"href,omitempty"`
}

// IPAddresses fields
type IPAddresses struct {
	ID            string     `json:"id,omitempty"`
	Address       string     `json:"address,omitempty"`
	AddressFamily int        `json:"address_family,omitempty"`
	Cidr          string     `json:"cidr,omitempty"`
	Gateway       string     `json:"gateway,omitempty"`
	Type          string     `json:"type,omitempty"`
	Region        Region     `json:"region,omitempty"`
	RoutedTo      RoutedTo   `json:"routed_to,omitempty"`
	AssignedTo    AssignedTo `json:"assigned_to,omitempty"`
	PtrRecord     string     `json:"ptr_record,omitempty"`
	ARecord       string     `json:"a_record,omitempty"`
	Href          string     `json:"href,omitempty"`
}

// RoutedTo fields
type RoutedTo struct {
	ID            string `json:"id,omitempty"`
	Address       string `json:"address,omitempty"`
	AddressFamily int    `json:"address_family,omitempty"`
	Cidr          string `json:"cidr,omitempty"`
	Gateway       string `json:"gateway,omitempty"`
	Type          string `json:"type,omitempty"`
	Region        Region `json:"region,omitempty"`
}

// AssignedTo fields
type AssignedTo struct {
	ID       int     `json:"id,omitempty"`
	Name     string  `json:"name,omitempty"`
	Href     string  `json:"href,omitempty"`
	Hostname string  `json:"hostname,omitempty"`
	Image    string  `json:"image,omitempty"`
	Region   Region  `json:"region,omitempty"`
	State    string  `json:"state,omitempty"`
	Pricing  Pricing `json:"pricing,omitempty"`
}

// ServersClient paveldi client
type ServersClient struct {
	client *Client
}

// List func lists teams
func (s *ServersClient) List(projectID string) ([]Servers, *Response, error) {
	//root := new(teamRoot)

	//serversIDString := strconv.Itoa(projectID)

	serversPath := strings.Join([]string{baseServersPath, projectID, endServersPath}, "/")

	var trans []Servers
	//resp := t.client.Bumba()
	//log.Println("\nFROM LIST1: ", root.Teams)
	resp, err := s.client.MakeRequest("GET", serversPath, nil, &trans)
	if err != nil {
		log.Fatal(err)
	}

	return trans, resp, err
}
