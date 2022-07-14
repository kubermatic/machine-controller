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

//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"fmt"
	"strconv"

	"google.golang.org/api/compute/v1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"

	v1 "k8s.io/api/core/v1"
)

// Possible instance statuses.
const (
	statusInstanceProvisioning = "PROVISIONING"
	statusInstanceRunning      = "RUNNING"
	statusInstanceStaging      = "STAGING"
	statusInstanceStopped      = "STOPPED"
	statusInstanceStopping     = "STOPPING"
	statusInstanceSuspended    = "SUSPENDED"
	statusInstanceSuspending   = "SUSPENDING"
	statusInstanceTerminated   = "TERMINATED"
)

// googleInstance implements instance.Instance for the Google compute instance.
type googleInstance struct {
	ci        *compute.Instance
	projectID string
	zone      string
}

// Name implements instance.Instance.
func (gi *googleInstance) Name() string {
	return gi.ci.Name
}

// ID implements instance.Instance.
func (gi *googleInstance) ID() string {
	return strconv.FormatUint(gi.ci.Id, 10)
}

func (gi *googleInstance) ProviderID() string {
	return fmt.Sprintf("gce://%s/%s/%s", gi.projectID, gi.zone, gi.ci.Name)
}

// Addresses implements instance.Instance.
func (gi *googleInstance) Addresses() map[string]v1.NodeAddressType {
	addrs := map[string]v1.NodeAddressType{}
	for _, ifc := range gi.ci.NetworkInterfaces {
		addrs[ifc.NetworkIP] = v1.NodeInternalIP
		for _, ac := range ifc.AccessConfigs {
			addrs[ac.NatIP] = v1.NodeExternalIP
		}
		for _, ac := range ifc.Ipv6AccessConfigs {
			addrs[ac.ExternalIpv6] = v1.NodeExternalIP
		}
	}

	// GCE has two types of the internal DNS, so we need to take both
	// into the account:
	// https://cloud.google.com/compute/docs/internal-dns#instance-fully-qualified-domain-names
	// Zonal DNS is present for newer projects and has the following FQDN format:
	// [INSTANCE_NAME].[ZONE].c.[PROJECT_ID].internal
	zonalDNS := fmt.Sprintf("%s.%s.c.%s.internal", gi.ci.Name, gi.zone, gi.projectID)
	addrs[zonalDNS] = v1.NodeInternalDNS

	// Global DNS is present for older projects and has the following FQDN format:
	// [INSTANCE_NAME].c.[PROJECT_ID].internal
	globalDNS := fmt.Sprintf("%s.c.%s.internal", gi.ci.Name, gi.projectID)
	addrs[globalDNS] = v1.NodeInternalDNS

	// GCP provides the search paths to resolve the machine's name,
	// so we add is as a DNS name
	// https://cloud.google.com/compute/docs/internal-dns#resolv.conf
	addrs[gi.ci.Name] = v1.NodeInternalDNS

	return addrs
}

// Status implements instance.Instance.
// TODO Check status mapping for staging, delet(ed|ing), suspend(ed|ing).
func (gi *googleInstance) Status() instance.Status {
	switch gi.ci.Status {
	case statusInstanceProvisioning:
		return instance.StatusCreating
	case statusInstanceRunning:
		return instance.StatusRunning
	case statusInstanceStaging:
		return instance.StatusCreating
	case statusInstanceStopped:
		return instance.StatusDeleted
	case statusInstanceStopping:
		return instance.StatusDeleting
	case statusInstanceSuspended:
		return instance.StatusDeleted
	case statusInstanceSuspending:
		return instance.StatusDeleting
	case statusInstanceTerminated:
		return instance.StatusDeleted
	}
	// Must not happen.
	return instance.StatusUnknown
}
