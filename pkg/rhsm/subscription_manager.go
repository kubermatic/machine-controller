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

package rhsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"k8s.io/klog"
)

const defaultTimeout = 10 * time.Second

// RedHatSubscriptionManager is responsible for removing redhat subscriptions.
type RedHatSubscriptionManager interface {
	UnregisterInstance(ctx context.Context, offlineToken, machineName string) error
}

type pagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Count  int `json:"count"`
}

type body struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type systemsResponse struct {
	Pagination pagination `json:"pagination"`
	Body       []body     `json:"body"`
}

type defaultRedHatSubscriptionManager struct {
	apiURL          string
	authURL         string
	requestsLimiter int
}

var errUnauthenticatedRequest = errors.New("unauthenticated")

func NewRedHatSubscriptionManager() RedHatSubscriptionManager {
	return &defaultRedHatSubscriptionManager{
		apiURL:          "https://api.access.redhat.com/management/v1/systems",
		authURL:         "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
		requestsLimiter: 100,
	}
}

func newOAuthClientWithRefreshToken(ctx context.Context, refreshToken string, tokenURL string) *http.Client {
	// Use the custom HTTP client when requesting an access token in order to
	// set a timeout value.
	// See: https://github.com/golang/oauth2/blob/c85d3e98c914e3a33234ad863dcbff5dbc425bb8/internal/token.go#L232
	httpClient := &http.Client{Timeout: defaultTimeout}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	conf := &oauth2.Config{
		ClientID: "rhsm-api",
		Endpoint: oauth2.Endpoint{
			TokenURL: tokenURL,
		},
	}
	tok := &oauth2.Token{
		RefreshToken: refreshToken,
	}
	// Set timeout for the client used for API calls as well.
	c := conf.Client(ctx, tok)
	c.Timeout = defaultTimeout
	return c
}

func (d *defaultRedHatSubscriptionManager) UnregisterInstance(ctx context.Context, offlineToken, machineName string) error {
	var (
		retries    = 0
		maxRetries = 15
	)

	for retries < maxRetries {
		machineUUID, err := d.findSystemsProfile(ctx, offlineToken, machineName)
		if err != nil {
			return fmt.Errorf("failed to find system profile: %w", err)
		}

		if machineUUID == "" {
			klog.Infof("machine uuid %s is not found", machineUUID)
			return nil
		}

		err = d.deleteSubscription(ctx, machineUUID, offlineToken)
		if err == nil {
			klog.Infof("subscription for vm %v has been deleted successfully", machineUUID)
			return nil
		}

		klog.Errorf("failed to delete subscription for system: %s due to: %v", machineUUID, err)
		time.Sleep(2 * time.Second)
		retries++
	}

	return errors.New("failed to delete system profile after max retires number has been reached")
}

func (d *defaultRedHatSubscriptionManager) findSystemsProfile(ctx context.Context, offlineToken, name string) (string, error) {
	var offset int
	for {
		systemsInfo, err := d.executeFindSystemsRequest(ctx, offlineToken, offset)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve systems: %w", err)
		}

		for _, system := range systemsInfo.Body {
			if system.Name == name {
				return system.UUID, nil
			}
		}

		if len(systemsInfo.Body) < d.requestsLimiter {
			break
		}

		offset += len(systemsInfo.Body)
	}

	klog.Infof("no machine name %s is found", name)
	return "", nil
}

func (d *defaultRedHatSubscriptionManager) deleteSubscription(ctx context.Context, uuid, offlineToken string) error {
	client := newOAuthClientWithRefreshToken(ctx, offlineToken, d.authURL)
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", d.apiURL, uuid), nil)
	if err != nil {
		return fmt.Errorf("failed to create delete system request: %w", err)
	}

	res, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to delete system profile: %w", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed while reading response: %w", err)
	}

	if res.StatusCode != http.StatusNoContent {
		if res.StatusCode == http.StatusUnauthorized {
			return errUnauthenticatedRequest
		}

		return fmt.Errorf("error while executing request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	return nil
}

func (d *defaultRedHatSubscriptionManager) executeFindSystemsRequest(ctx context.Context, offlineToken string, offset int) (*systemsResponse, error) {
	client := newOAuthClientWithRefreshToken(ctx, offlineToken, d.authURL)
	req, err := http.NewRequest("GET", fmt.Sprintf(d.apiURL+"?limit=%v&offset=%v", d.requestsLimiter, offset), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch systems request: %w", err)
	}
	res, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed executing fetch systems request: %w", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed while reading response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return nil, errUnauthenticatedRequest
		}
		return nil, fmt.Errorf("error while executing request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	var fetchedSystems = &systemsResponse{}
	if err := json.Unmarshal(data, fetchedSystems); err != nil {
		return nil, fmt.Errorf("failed while unmarshalling data: %w", err)
	}

	return fetchedSystems, nil
}
