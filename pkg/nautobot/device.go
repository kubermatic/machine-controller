/*
Copyright 2021 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nautobot

import "time"

type DeviceStatus string

const (
	Active  DeviceStatus = "active"
	Planned DeviceStatus = "planned"
	Staged  DeviceStatus = "staged"
)

type NetworkDevice struct {
	Count   int          `json:"count"`
	Results []DeviceInfo `json:"results"`
}

type DeviceInfo struct {
	ID               string         `json:"id"`
	URL              string         `json:"url"`
	Name             string         `json:"name"`
	DisplayName      string         `json:"display_name"`
	DeviceType       *DeviceType    `json:"device_type"`
	DeviceRole       *DeviceRole    `json:"device_role"`
	Tenant           *Tenant        `json:"tenant"`
	Platform         *Platform      `json:"platform"`
	Serial           string         `json:"serial"`
	AssetTag         interface{}    `json:"asset_tag"`
	Site             *Site          `json:"site"`
	Rack             *Rack          `json:"rack"`
	Position         int            `json:"position"`
	Face             *Face          `json:"face"`
	ParentDevice     interface{}    `json:"parent_device"`
	Status           *Status        `json:"status"`
	PrimaryIP        *PrimaryIP     `json:"primary_ip"`
	PrimaryIP4       *PrimaryIP4    `json:"primary_ip4"`
	PrimaryIP6       interface{}    `json:"primary_ip6"`
	Cluster          interface{}    `json:"cluster"`
	VirtualChassis   interface{}    `json:"virtual_chassis"`
	VcPosition       interface{}    `json:"vc_position"`
	VcPriority       interface{}    `json:"vc_priority"`
	Comments         string         `json:"comments"`
	LocalContextData interface{}    `json:"local_context_data"`
	Tags             []Tags         `json:"tags"`
	ConfigContext    *ConfigContext `json:"config_context"`
	Created          string         `json:"created"`
	LastUpdated      time.Time      `json:"last_updated"`
}

type Manufacturer struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type DeviceType struct {
	ID           string        `json:"id"`
	URL          string        `json:"url"`
	Manufacturer *Manufacturer `json:"manufacturer"`
	Model        string        `json:"model"`
	Slug         string        `json:"slug"`
	DisplayName  string        `json:"display_name"`
}

type DeviceRole struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Platform struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Rack struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type Face struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type PrimaryIP struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Family  int    `json:"family"`
	Address string `json:"address"`
}

type PrimaryIP4 struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Family  int    `json:"family"`
	Address string `json:"address"`
}

type Ntp []struct {
	IP     string `json:"ip"`
	Prefer bool   `json:"prefer"`
}

type Host []struct {
	IP        string `json:"ip"`
	Version   string `json:"version"`
	Community string `json:"community"`
}

type Community []struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type Snmp struct {
	Host      Host      `json:"host"`
	Contact   string    `json:"contact"`
	Location  string    `json:"location"`
	Community Community `json:"community"`
}

type Named struct {
	PermitRoutes []string `json:"PERMIT_ROUTES"`
}

type Definitions struct {
	Named *Named `json:"named"`
}

type ACL struct {
	Definitions *Definitions `json:"definitions"`
}

type PermitConnRoutes struct {
	Seq        int      `json:"seq"`
	Type       string   `json:"type"`
	Statements []string `json:"statements"`
}

type RouteMaps struct {
	PermitConnRoutes *PermitConnRoutes `json:"PERMIT_CONN_ROUTES"`
}

type ConfigContext struct {
	Cdp         bool      `json:"cdp"`
	Ntp         Ntp       `json:"ntp"`
	Lldp        bool      `json:"lldp"`
	Snmp        *Snmp     `json:"snmp"`
	AaaNewModel bool      `json:"aaa-new-model"`
	ACL         *ACL      `json:"acl"`
	RouteMaps   RouteMaps `json:"route-maps"`
}
