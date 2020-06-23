package rhsm

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"k8s.io/klog"
)

// SatelliteSubscriptionManager manages the communications between machine-controller and redhat satellite server
type SatelliteSubscriptionManager interface {
	DeleteSatelliteHost(machineName, username, password, serverURL string) error
}

// DefaultSatelliteSubscriptionManager default manager for redhat satellite server.
type DefaultSatelliteSubscriptionManager struct {
	client *http.Client
}

// NewSatelliteSubscriptionManager creates a new Redhat satellite manager.
func NewSatelliteSubscriptionManager() SatelliteSubscriptionManager {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := http.DefaultClient
	client.Transport = transport

	return &DefaultSatelliteSubscriptionManager{
		client: client,
	}
}

func (s *DefaultSatelliteSubscriptionManager) DeleteSatelliteHost(machineName, username, password, serverURL string) error {
	if machineName == "" || username == "" || password == "" || serverURL == "" {
		return errors.New("satellite server url, username or password cannot be empty")
	}

	var requestURL url.URL
	requestURL.Scheme = "https"
	requestURL.Host = serverURL
	requestURL.Path = path.Join("api", "hosts", machineName)

	deleteHostRequest, err := http.NewRequest(http.MethodDelete, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create a delete host request: %v", err)
	}

	deleteHostRequest.SetBasicAuth(username, password)

	var (
		retries    = 0
		maxRetries = 15
		response   = &http.Response{}
	)

	for retries < maxRetries {
		response, err = s.client.Do(deleteHostRequest)
		if err != nil {
			klog.Errorf("failed executing delete host request: %v", err)
			retries++
			continue
		}

		if response.StatusCode != http.StatusOK {
			klog.Errorf("error while executing request with status code: %v", response.StatusCode)
			retries++
			continue
		}

		klog.Infof("host %v has been deleted successfully", machineName)
		response.Body.Close()
		return nil
	}

	response.Body.Close()
	return errors.New("failed to delete system profile after max retires number has been reached")
}
