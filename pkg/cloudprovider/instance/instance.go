/*
Copyright 2019 The Machine Controller Authors.

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

package instance

import v1 "k8s.io/api/core/v1"

// Instance represents a instance on the cloud provider.
type Instance interface {
	// Name returns the instance name.
	Name() string
	// ID returns the instance identifier.
	ID() string
	// ProviderID returns the expected providerID for the instance
	ProviderID() string
	// Addresses returns a list of addresses associated with the instance.
	Addresses() map[string]v1.NodeAddressType
	// Status returns the instance status.
	Status() Status
}

// Status represents the instance status.
type Status string

const (
	StatusRunning  Status = "running"
	StatusDeleting Status = "deleting"
	StatusDeleted  Status = "deleted"
	StatusCreating Status = "creating"
	StatusUnknown  Status = "unknown"
)
