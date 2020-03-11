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
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"k8s.io/klog"
)

// RedHatSubscriptionManager is responsible for removing redhat subscriptions.
type RedHatSubscriptionManager interface {
	//TODO(irozzo) add context in input to give more control to the caller
	UnregisterInstance(machineName string) error
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
	client          *http.Client
	requestsLimiter int
}

var errUnauthenticatedRequest = errors.New("unauthenticated")

func NewRedHatSubscriptionManager(offlineToken string) (RedHatSubscriptionManager, error) {
	if offlineToken == "" {
		return nil, errors.New("RedHatSubscriptionManager offline token cannot be empty")
	}
	return &defaultRedHatSubscriptionManager{
		apiURL: "https://api.access.redhat.com/management/v1/systems",
		client: newOAuthClientWithRefreshToken(offlineToken, "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"),
	}, nil
}

func newOAuthClientWithRefreshToken(refreshToken string, tokenURL string) *http.Client {
	ctx := context.Background()
	// Use the custom HTTP client when requesting a token.
	httpClient := &http.Client{Timeout: 5 * time.Second}
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
	return conf.Client(ctx, tok)
}

func (d *defaultRedHatSubscriptionManager) UnregisterInstance(machineName string) error {
	ctx := context.Background()

	var (
		retries    = 0
		maxRetries = 15
	)

	for retries < maxRetries {
		machineUUID, err := d.findSystemsProfile(ctx, machineName)
		if err != nil {
			return fmt.Errorf("failed to find system profile: %v", err)
		}

		if machineUUID == "" {
			klog.Infof("machine uuid %s is not found", machineUUID)
			return nil
		}

		err = d.deleteSubscription(ctx, machineUUID)
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

func (d *defaultRedHatSubscriptionManager) findSystemsProfile(ctx context.Context, name string) (string, error) {
	var offset int
	for {
		systemsInfo, err := d.executeFindSystemsRequest(ctx, offset)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve systems: %v", err)
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

func (d *defaultRedHatSubscriptionManager) deleteSubscription(ctx context.Context, uuid string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", d.apiURL, uuid), nil)
	if err != nil {
		return fmt.Errorf("failed to create delete system request: %v", err)
	}
	req.WithContext(ctx)

	res, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("faild to delete systsem profile: %v", err)
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed while reading response: %v", err)
	}

	if res.StatusCode != http.StatusNoContent {
		if res.StatusCode == http.StatusUnauthorized {
			return errUnauthenticatedRequest
		}

		return fmt.Errorf("error while executing request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	return nil
}

func (d *defaultRedHatSubscriptionManager) executeFindSystemsRequest(ctx context.Context, offset int) (*systemsResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(d.apiURL+"?limit=%v&offset=%v", d.requestsLimiter, offset), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch systems request: %v", err)
	}
	req.WithContext(ctx)

	res, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed executing fetch systems request: %v", err)
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed while reading response: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return nil, errUnauthenticatedRequest
		}
		return nil, fmt.Errorf("error while executing request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	var fetchedSystems = &systemsResponse{}
	if err := json.Unmarshal(data, fetchedSystems); err != nil {
		return nil, fmt.Errorf("failed while unmarshalling data: %v", err)
	}

	return fetchedSystems, nil
}
