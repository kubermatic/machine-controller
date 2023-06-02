/*
Copyright 2020 The Machine Controller Authors.

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

package rhsm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"k8s.io/klog"
)

// SatelliteSubscriptionManager manages the communications between machine-controller and redhat satellite server.
type SatelliteSubscriptionManager interface {
	DeleteSatelliteHost(ctx context.Context, machineName, username, password, serverURL string) error
}

// DefaultSatelliteSubscriptionManager default manager for redhat satellite server.
type DefaultSatelliteSubscriptionManager struct {
	client *http.Client

	useHTTP bool
}

// NewSatelliteSubscriptionManager creates a new Redhat satellite manager.
func NewSatelliteSubscriptionManager() SatelliteSubscriptionManager {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSHandshakeTimeout: defaultTimeout,
		},
		Timeout: defaultTimeout,
	}

	return &DefaultSatelliteSubscriptionManager{
		client: client,
	}
}

func (s *DefaultSatelliteSubscriptionManager) DeleteSatelliteHost(ctx context.Context, machineName, username, password, serverURL string) error {
	if machineName == "" || username == "" || password == "" || serverURL == "" {
		return errors.New("satellite server url, username or password cannot be empty")
	}

	var (
		retries    = 0
		maxRetries = 15
	)

	for retries < maxRetries {
		if err := s.executeDeleteRequest(ctx, machineName, username, password, serverURL); err != nil {
			klog.Errorf("failed to execute satellite subscription deletion: %v", err)
			retries++
			time.Sleep(500 * time.Second)
			continue
		}

		klog.Infof("subscription for machine %s deleted successfully", machineName)
		return nil
	}

	return errors.New("failed to delete system profile after max retires number has been reached")
}

func (s *DefaultSatelliteSubscriptionManager) executeDeleteRequest(ctx context.Context, machineName, username, password, serverURL string) error {
	var requestURL url.URL
	requestURL.Scheme = "http"
	if !s.useHTTP {
		requestURL.Scheme = "https"
	}
	requestURL.Host = serverURL
	requestURL.Path = path.Join("api", "v2", "hosts", machineName)

	deleteHostRequest, err := http.NewRequest(http.MethodDelete, requestURL.String(), nil)
	deleteHostRequest = deleteHostRequest.WithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to create a delete host request: %w", err)
	}

	deleteHostRequest.SetBasicAuth(username, password)

	response, err := s.client.Do(deleteHostRequest)
	if err != nil {
		return fmt.Errorf("failed executing delete host request: %w", err)
	}
	defer response.Body.Close()

	if !validStatusCode(response.StatusCode) {
		return fmt.Errorf("error while executing request with status code: %v", response.StatusCode)
	}

	klog.Infof("host %v has been deleted successfully", machineName)
	return nil
}

func validStatusCode(status int) bool {
	switch status {
	case http.StatusOK:
		return true
	case http.StatusNotFound:
		return true
	}

	return false
}
