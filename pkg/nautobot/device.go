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
	Active DeviceStatus = "active"
	Staged DeviceStatus = "staged"
)

type NetworkDevice struct {
	Count   int           `json:"count,omitempty"`
	Results []*DeviceInfo `json:"results,omitempty"`
}

type DeviceInfo struct {
	ID               string         `json:"id,omitempty"`
	URL              string         `json:"url,omitempty"`
	Name             string         `json:"name,omitempty"`
	DisplayName      string         `json:"display_name,omitempty"`
	DeviceType       *DeviceType    `json:"device_type,omitempty"`
	DeviceRole       *DeviceRole    `json:"device_role,omitempty"`
	Tenant           *Tenant        `json:"tenant,omitempty"`
	Platform         *Platform      `json:"platform,omitempty"`
	Serial           string         `json:"serial,omitempty"`
	AssetTag         interface{}    `json:"asset_tag,omitempty"`
	Site             *Site          `json:"site,omitempty"`
	Rack             *Rack          `json:"rack,omitempty"`
	Position         int            `json:"position,omitempty"`
	Face             *Face          `json:"face,omitempty"`
	ParentDevice     interface{}    `json:"parent_device,omitempty"`
	Status           *Status        `json:"status,omitempty"`
	PrimaryIP        *PrimaryIP     `json:"primary_ip,omitempty"`
	PrimaryIP4       *PrimaryIP4    `json:"primary_ip4,omitempty"`
	PrimaryIP6       interface{}    `json:"primary_ip6,omitempty"`
	Cluster          interface{}    `json:"cluster,omitempty"`
	VirtualChassis   interface{}    `json:"virtual_chassis,omitempty"`
	VcPosition       interface{}    `json:"vc_position,omitempty"`
	VcPriority       interface{}    `json:"vc_priority,omitempty"`
	Comments         string         `json:"comments,omitempty"`
	LocalContextData interface{}    `json:"local_context_data,omitempty"`
	Tags             []Tags         `json:"tags,omitempty"`
	ConfigContext    *ConfigContext `json:"config_context,omitempty"`
	Created          string         `json:"created,omitempty"`
	LastUpdated      time.Time      `json:"last_updated,omitempty"`
}

type Manufacturer struct {
	ID   string `json:"id,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type DeviceType struct {
	ID           string        `json:"id,omitempty"`
	URL          string        `json:"url,omitempty"`
	Manufacturer *Manufacturer `json:"manufacturer,omitempty"`
	Model        string        `json:"model,omitempty"`
	Slug         string        `json:"slug,omitempty"`
	DisplayName  string        `json:"display_name,omitempty"`
}

type DeviceRole struct {
	ID   string `json:"id,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type Platform struct {
	ID   string `json:"id,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type Rack struct {
	ID          string `json:"id,omitempty"`
	URL         string `json:"url,omitempty"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type Face struct {
	Value string `json:"value,omitempty"`
	Label string `json:"label,omitempty"`
}

type PrimaryIP struct {
	ID      string `json:"id,omitempty"`
	URL     string `json:"url,omitempty"`
	Family  int    `json:"family,omitempty"`
	Address string `json:"address,omitempty"`
}

type PrimaryIP4 struct {
	ID      string `json:"id,omitempty"`
	URL     string `json:"url,omitempty"`
	Family  int    `json:"family,omitempty"`
	Address string `json:"address,omitempty"`
}

type Ntp []struct {
	IP     string `json:"ip,omitempty"`
	Prefer bool   `json:"prefer,omitempty"`
}

type Host []struct {
	IP        string `json:"ip,omitempty"`
	Version   string `json:"version,omitempty"`
	Community string `json:"community,omitempty"`
}

type Community []struct {
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"`
}

type Snmp struct {
	Host      Host      `json:"host,omitempty"`
	Contact   string    `json:"contact,omitempty"`
	Location  string    `json:"location,omitempty"`
	Community Community `json:"community,omitempty"`
}

type Named struct {
	PermitRoutes []string `json:"PERMIT_ROUTES,omitempty"`
}

type Definitions struct {
	Named *Named `json:"named,omitempty"`
}

type ACL struct {
	Definitions *Definitions `json:"definitions,omitempty"`
}

type PermitConnRoutes struct {
	Seq        int      `json:"seq,omitempty"`
	Type       string   `json:"type,omitempty"`
	Statements []string `json:"statements,omitempty"`
}

type RouteMaps struct {
	PermitConnRoutes *PermitConnRoutes `json:"PERMIT_CONN_ROUTES,omitempty"`
}

type ConfigContext struct {
	Cdp         bool      `json:"cdp,omitempty"`
	Ntp         Ntp       `json:"ntp,omitempty"`
	Lldp        bool      `json:"lldp,omitempty"`
	Snmp        *Snmp     `json:"snmp,omitempty"`
	AaaNewModel bool      `json:"aaa-new-model,omitempty"`
	ACL         *ACL      `json:"acl,omitempty"`
	RouteMaps   RouteMaps `json:"route-maps,omitempty"`
}

type PatchedDeviceParams struct {
	Status   DeviceStatus `json:"status"`
	AssetTag string       `json:"asset_tag"`
}
