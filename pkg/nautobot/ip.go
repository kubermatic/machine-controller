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

type IP struct {
	Count   int      `json:"count"`
	Results []IPInfo `json:"results"`
}

type IPInfo struct {
	ID                 string          `json:"id"`
	URL                string          `json:"url"`
	Family             *Family         `json:"family"`
	Address            string          `json:"address"`
	Vrf                *Vrf            `json:"vrf"`
	Tenant             *Tenant         `json:"tenant"`
	Status             *Status         `json:"status"`
	AssignedObjectType string          `json:"assigned_object_type"`
	AssignedObjectID   string          `json:"assigned_object_id"`
	AssignedObject     *AssignedObject `json:"assigned_object"`
	DNSName            string          `json:"dns_name"`
	Description        string          `json:"description"`
	Created            string          `json:"created"`
	LastUpdated        time.Time       `json:"last_updated"`
}

type AssignedObject struct {
	ID     string  `json:"id"`
	URL    string  `json:"url"`
	Device *Device `json:"device"`
	Name   string  `json:"name"`
	Cable  string  `json:"cable"`
}
