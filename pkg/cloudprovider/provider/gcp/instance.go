//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"strconv"

	"google.golang.org/api/compute/v1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
)

//-----
// Constants
//-----

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

//-----
// Instance
//-----

// instance implements instance.Instance for the GCP instance.
type instance struct {
	ci *compute.Instance
}

// Name implements instance.Instance.
func (i *instance) Name() string {
	return i.ci.Name
}

// ID implements instance.Instance.
func (i *instance) ID() string {
	return strconv.FormatUint(i.ci.Id, 10)
}

// Addresses implements instance.Instance.
func (i *instance) Addresses() []string {
	var addrs []string
	for _, ifc := range i.ci.NetworkInterfaces {
		addrs = append(addrs, ifc.NetworkIP)
	}
	return addrs
}

// Status implements instance.Instance.
// TODO Check status mapping for staging, delet(ed|ing), suspend(ed|ing).
func (i *instance) Status() instance.Status {
	switch i.ci.Status {
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
