//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
)

//-----
// Constants
//-----

const (
	// connectionType describes the kind of GCP connection.
	connectionType = "service_account"

	// Timeouts.
	timeoutNormal = 5 * time.Second
)

// GCP operation status.
const (
	statusDone         = "DONE"
	statusDown         = "DOWN"
	statusPending      = "PENDING"
	statusProvisioning = "PROVISIONING"
	statusRunning      = "RUNNING"
	statusStaging      = "STAGING"
	statusStopped      = "STOPPED"
	statusStopping     = "STOPPING"
	statusTerminated   = "TERMINATED"
	statusUp           = "UP"
)

//-----
// Service
//-----

// service wraps a GCP compute service for the extension with helper methods.
type service struct {
	*compute.Service
}

// connectComputeService establishes a service connection to the Compute Engine.
func connectComputeService(cfg *config) (*service, error) {
	conf := &jwt.Config{
		Email:      cfg.email,
		PrivateKey: cfg.privateKey,
		Scopes: []string{
			compute.ComputeScope,
		},
		TokenURL: google.JWTTokenURL,
	}
	svc, err := compute.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud Platform: %v", err)
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
			DiskType:    cfg.diskType,
			SourceImage: sourceImage,
		},
	}
	return []*compute.AttachedDisk{bootDisk}, nil
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

// waitOperation waits for a GCP operation to be completed or timed out.
func (svc *service) waitOperation(projectID string, op *compute.Operation, timeout time.Duration) (err error) {
	started := time.Now()
	waiting := 100 * time.Millisecond
	for {
		// Check if done (successfully).
		if op.Status == statusDone {
			if op.Error != nil {
				// Operation failed.
				for _, err := range op.Error.Errors {
					glog.Errorf("GCP operation %q error: (%s) %s", op.Name, err.Code, err.Message)
				}
				return fmt.Errorf("GCP operation %q failed", op.Name)
			}
			return nil
		}
		// If not done grant some growing time.
		if time.Now().Sub(started) > timeout {
			// Operation timed out.
			return fmt.Errorf("GCP operation %q timed out after %d seconds", op.Name, time.Now().Sub(started)/time.Second)
		}
		time.Sleep(waiting)
		waiting = waiting * 2
		// Refresh operation to gather new status.
		op, err = svc.refreshOperation(projectID, op)
		if err != nil {
			return fmt.Errorf("GCP operation %q refreshing failed: %v", op.Name, err)
		}
	}
}
