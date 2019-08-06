package cherrygo

import (
	"fmt"
	"strconv"
	"strings"
)

// GetProject interface metodas isgauti team'sus
type GetProject interface {
	List(projectID string) (Project, *Response, error)
	Create(teamID int, request *CreateProject) (Project, *Response, error)
	Update(projectID string, request *UpdateProject) (Project, *Response, error)
	Delete(projectID string, request *DeleteProject) (Project, *Response, error)
}

// Project tai ka grazina api
type Project struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Href string `json:"href,omitempty"`
}

// ProjectClient paveldi client
type ProjectClient struct {
	client *Client
}

// CreateProject fields for adding new project with specified name
type CreateProject struct {
	Name string `json:"name,omitempty"`
}

// UpdateProject fields for updating a project with specified name
type UpdateProject struct {
	Name string `json:"name,omitempty"`
}

// DeleteProject fields for key delition by its ID
type DeleteProject struct {
	ID string `json:"id,omitempty"`
}

// List project
func (p *ProjectClient) List(projectID string) (Project, *Response, error) {

	projectPath := strings.Join([]string{baseIPSPath, projectID}, "/")

	var trans Project
	//resp := t.client.Bumba()
	//log.Println("\nFROM LIST1: ", root.Teams)
	resp, err := p.client.MakeRequest("GET", projectPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}

// Create func will create new Project for specified team
func (p *ProjectClient) Create(teamID int, request *CreateProject) (Project, *Response, error) {

	teamIDString := strconv.Itoa(teamID)

	var trans Project

	plansPath := strings.Join([]string{baseProjectPath, teamIDString, endProjectPath}, "/")

	resp, err := p.client.MakeRequest("POST", plansPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}

// Update func will update a project
func (p *ProjectClient) Update(projectID string, request *UpdateProject) (Project, *Response, error) {

	var trans Project

	projectPath := strings.Join([]string{baseIPSPath, projectID}, "/")

	resp, err := p.client.MakeRequest("PUT", projectPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}

// Delete func will delete a project
func (p *ProjectClient) Delete(projectID string, request *DeleteProject) (Project, *Response, error) {

	var trans Project

	projectPath := strings.Join([]string{baseIPSPath, projectID}, "/")

	resp, err := p.client.MakeRequest("DELETE", projectPath, request, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}
	return trans, resp, err
}
