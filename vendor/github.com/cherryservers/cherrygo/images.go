package cherrygo

import (
	"fmt"
	"strconv"
	"strings"
)

const baseImagePath = "/v1/plans"
const endImagePath = "images"

// GetImages interface metodas isgauti team'sus
type GetImages interface {
	List(planID int) ([]Images, *Response, error)
}

// Images tai ka grazina api
type Images struct {
	ID      int       `json:"id,omitempty"`
	Name    string    `json:"name,omitempty"`
	Pricing []Pricing `json:"pricing,omitempty"`
}

// ImagesClient paveldi client
type ImagesClient struct {
	client *Client
}

// List func lists teams
func (i *ImagesClient) List(planID int) ([]Images, *Response, error) {
	//root := new(teamRoot)

	planIDString := strconv.Itoa(planID)

	plansPath := strings.Join([]string{baseImagePath, planIDString, endImagePath}, "/")

	var trans []Images

	resp, err := i.client.MakeRequest("GET", plansPath, nil, &trans)
	if err != nil {
		err = fmt.Errorf("Error: %v", err)
	}

	return trans, resp, err
}
