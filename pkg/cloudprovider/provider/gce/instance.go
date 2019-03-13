//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"strconv"

	"google.golang.org/api/compute/v1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
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
	ci *compute.Instance
}

// Name implements instance.Instance.
func (gi *googleInstance) Name() string {
	return gi.ci.Name
}

// ID implements instance.Instance.
func (gi *googleInstance) ID() string {
	return strconv.FormatUint(gi.ci.Id, 10)
}

// Addresses implements instance.Instance.
func (gi *googleInstance) Addresses() []string {
	var addrs []string
	for _, ifc := range gi.ci.NetworkInterfaces {
		addrs = append(addrs, ifc.NetworkIP)
	}
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
