//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"fmt"
	"path"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// Interval and timeout for polling.
	pollInterval = 5 * time.Second
	pollTimeout  = 5 * time.Minute
)

// Google compute operation status.
const (
	statusDone = "DONE"
)

// service wraps a GCE compute service for the extension with helper methods.
type service struct {
	*compute.Service
}

// connectComputeService establishes a service connection to the Compute Engine.
func connectComputeService(cfg *config) (*service, error) {
	svc, err := compute.New(cfg.jwtConfig.Client(oauth2.NoContext))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud: %v", err)
	}
	return &service{svc}, nil
}

// networkInterfaces returns the configured network interfaces for an instance creation.
func (svc *service) networkInterfaces(cfg *config) ([]*compute.NetworkInterface, error) {
	ifc := &compute.NetworkInterface{
		AccessConfigs: []*compute.AccessConfig{
			{
				Name: "External NAT",
				Type: "ONE_TO_ONE_NAT",
			},
		},
		Network: "global/networks/default",
	}
	return []*compute.NetworkInterface{ifc}, nil
}

// attachedDisks returns the configured attached disks for an instance creation.
func (svc *service) attachedDisks(cfg *config) ([]*compute.AttachedDisk, error) {
	sourceImage, err := cfg.sourceImageDescriptor()
	if err != nil {
		return nil, err
	}
	bootDisk := &compute.AttachedDisk{
		Boot:       true,
		AutoDelete: true,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskSizeGb:  cfg.diskSize,
			DiskType:    cfg.diskTypeDescriptor(),
			SourceImage: sourceImage,
		},
	}
	return []*compute.AttachedDisk{bootDisk}, nil
}

// waitOperation waits for a GCE operation to be completed or timed out.
func (svc *service) waitOperation(projectID string, op *compute.Operation) error {
	return wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		// Check if done (successfully).
		if op.Status == statusDone {
			if op.Error != nil {
				// Operation failed.
				for _, err := range op.Error.Errors {
					glog.Errorf("GCE operation %q error: (%s) %s", op.Name, err.Code, err.Message)
				}
				return false, fmt.Errorf("GCE operation %q failed", op.Name)
			}
			return true, nil
		}
		// Refresh operation to gather new status.
		var err error
		op, err = svc.refreshOperation(projectID, op)
		if err != nil {
			return false, fmt.Errorf("GCE operation %q refreshing failed: %v", op.Name, err)
		}
		// Not yet done.
		return false, nil
	})
}

// refreshOperation requests a fresh copy of the passed operation containing
// the updated status.
func (svc *service) refreshOperation(projectID string, op *compute.Operation) (*compute.Operation, error) {
	switch {
	case op.Zone != "":
		zone := path.Base(op.Zone)
		return svc.ZoneOperations.Get(projectID, zone, op.Name).Do()
	case op.Region != "":
		region := path.Base(op.Region)
		return svc.RegionOperations.Get(projectID, region, op.Name).Do()
	default:
		return svc.GlobalOperations.Get(projectID, op.Name).Do()
	}
}
