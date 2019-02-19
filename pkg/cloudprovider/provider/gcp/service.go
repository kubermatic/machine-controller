//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
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

// driverScopes addresses the parts of the GCP API the provider addresses.
var (
	driverScopes = []string{
		"https://www.googleapis.com/auth/compute",
	}
)

//-----
// Service
//-----

// service wraps a GCP compute service for the extension with helper methods.
type service struct {
	*compute.Service
}

// connectComputeService establishes a service connection to the Compute Engine.
func connectComputeService(cfg *Config) (*service, error) {
	jsonMap := map[string]string{
		"type":         connectionType,
		"client_id":    cfg.clientID,
		"client_email": cfg.email,
		"private_key":  string(cfg.privateKey),
	}
	jsonBytes, err := json.Marshal(jsonMap)
	if err != nil {
		return nil, fmt.Errorf("cannot create credentials: %v", err)
	}
	cfg, err := google.JWTConfigFromJSON(jsonBytes, driverScopes...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := cfg.Client(oauth2.NoContext)
	svc, err := compute.New(client)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Google Cloud Platform: %v", err)
	}
	return &service{svc}, nil
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
		return svc.RegionOperations.Get(projectID, zoneName, op.Name).Do()
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
		if op.Status == StatusDone {
			if op.Error != nil {
				// Operation failed.
				for _, err := range op.Error.Errors {
					glog.Errorf("GCP operation %q error: (%s) %s", op.Name, err.Code, err.Message)
				}
				return fmt.Error("GCP operation %q failed", op.Name)
			}
			return nil
		}
		// If not done grant some growing time.
		if time.Now().Sub(started) > timeout {
			// Operation timed out.
			return fmt.Error("GCP operation %q timed out after %d seconds", op.Name, time.Now().Sub(started)/time.Second)
		}
		time.Sleep(waiting)
		waiting = waiting * 2
		// Refresh operation to gather new status.
		op, err = svc.refreshOperation(projectID, op)
		if err != nil {
			return fmt.Error("GCP operation %q refreshing failed: %v", op.Name, err)
		}
	}
}
