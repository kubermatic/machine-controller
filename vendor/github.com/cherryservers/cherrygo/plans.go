package cherrygo

import (
	"log"
	"strconv"
	"strings"
)

const basePlanPath = "/v1/teams"
const endPlanPath = "plans"

// GetPlans interface metodas isgauti team'sus
type GetPlans interface {
	List(teamID int) ([]Plans, *Response, error)
}

// Plans tai ka grazina api
type Plans struct {
	ID               int                `json:"id,omitempty"`
	Name             string             `json:"name,omitempty"`
	Custom           bool               `json:"custom,omitempty"`
	Specs            Specs              `json:"specs,omitempty"`
	Pricing          []Pricing          `json:"pricing,omitempty"`
	AvailableRegions []AvailableRegions `json:"available_regions,omitempty"`
}

// Specs specifies fields for specs
type Specs struct {
	Cpus      Cpus      `json:"cpus,omitempty"`
	Memory    Memory    `json:"memory,omitempty"`
	Storage   []Storage `json:"storage,omitempty"`
	Raid      Raid      `json:"raid,omitempty"`
	Nics      Nics      `json:"nics,omitempty"`
	Bandwidth Bandwidth `json:"bandwidth,omitempty"`
}

// Cpus fields
type Cpus struct {
	Count     int     `json:"count,omitempty"`
	Name      string  `json:"name,omitempty"`
	Cores     int     `json:"cores,omitempty"`
	Frequency float32 `json:"frequency,omitempty"`
	Unit      string  `json:"unit,omitempty"`
}

// Memory fields
type Memory struct {
	Count int    `json:"count,omitempty"`
	Total int    `json:"total,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Storage fields
type Storage struct {
	Count int     `json:"count,omitempty"`
	Name  string  `json:"name,omitempty"`
	Size  float32 `json:"size,omitempty"`
	Unit  string  `json:"unit,omitempty"`
}

// Raid fields
type Raid struct {
	Name string `json:"name,omitempty"`
}

// Nics fields
type Nics struct {
	Name string `json:"name,omitempty"`
}

// Bandwidth fields
type Bandwidth struct {
	Name string `json:"name,omitempty"`
}

// Pricing2 specifies fields for specs
type Pricing2 struct {
	Price    float32 `json:"price,omitempty"`
	Currency string  `json:"currency,omitempty"`
	Taxed    bool    `json:"taxed,omitempty"`
	Unit     string  `json:"unit,omitempty"`
}

// AvailableRegions specifies fields for specs
type AvailableRegions struct {
	ID         int    `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	RegionIso2 string `json:"region_iso_2,omitempty"`
	StockQty   int    `json:"stock_qty,omitempty"`
}

// PlansClient paveldi client
type PlansClient struct {
	client *Client
}

// List func lists teams
func (p *PlansClient) List(teamID int) ([]Plans, *Response, error) {
	//root := new(teamRoot)

	teamIDString := strconv.Itoa(teamID)

	plansPath := strings.Join([]string{basePlanPath, teamIDString, endPlanPath}, "/")

	var trans []Plans
	//resp := t.client.Bumba()
	//log.Println("\nFROM LIST1: ", root.Teams)
	resp, err := p.client.MakeRequest("GET", plansPath, nil, &trans)
	if err != nil {
		log.Fatal(err)
	}

	return trans, resp, err
}
