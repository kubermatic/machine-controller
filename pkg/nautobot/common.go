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

type Status struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Device struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type Tags struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Color string `json:"color"`
}

type Tenant struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Vrf struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Rd          string `json:"rd"`
	DisplayName string `json:"display_name"`
}

type Family struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type Site struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
