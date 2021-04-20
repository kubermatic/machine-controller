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

type Interface struct {
	Count   int             `json:"count"`
	Results []InterfaceInfo `json:"results"`
}

type InterfaceInfo struct {
	ID                         string             `json:"id"`
	URL                        string             `json:"url"`
	Device                     *Device            `json:"device"`
	Name                       string             `json:"name"`
	Label                      string             `json:"label"`
	Type                       *Type              `json:"type"`
	Enabled                    bool               `json:"enabled"`
	MacAddress                 string             `json:"mac_address"`
	MgmtOnly                   bool               `json:"mgmt_only"`
	Description                string             `json:"description"`
	Cable                      *Cable             `json:"cable"`
	CablePeer                  *CablePeer         `json:"cable_peer"`
	CablePeerType              string             `json:"cable_peer_type"`
	ConnectedEndpoint          *ConnectedEndpoint `json:"connected_endpoint"`
	ConnectedEndpointType      string             `json:"connected_endpoint_type"`
	ConnectedEndpointReachable bool               `json:"connected_endpoint_reachable"`
	CountIpaddresses           int                `json:"count_ipaddresses"`
}

type Type struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Cable struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

type CablePeer struct {
	ID     string  `json:"id"`
	URL    string  `json:"url"`
	Device *Device `json:"device"`
	Name   string  `json:"name"`
	Cable  string  `json:"cable"`
}

type ConnectedEndpoint struct {
	ID     string  `json:"id"`
	URL    string  `json:"url"`
	Device *Device `json:"device"`
	Name   string  `json:"name"`
	Cable  string  `json:"cable"`
}
