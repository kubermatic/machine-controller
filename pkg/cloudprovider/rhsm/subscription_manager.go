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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RedHatSubscriptionManager is responsible for removing redhat subscriptions.
type RedHatSubscriptionManager interface {
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
	Body       []*body    `json:"body"`
}

type credentials struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type defaultRedHatSubscriptionManager struct {
	offlineToken string
	authURL      string
	apiURL       string
	client       *http.Client
	credentials  *credentials
}

var errUnauthenticatedRequest = errors.New("unauthenticated")

func NewRedHatSubscriptionManager(offlineToken string) (RedHatSubscriptionManager, error) {
	if offlineToken == "" {
		return nil, errors.New("offline token, authURL, or apiPath cannot be empty")
	}

	return &defaultRedHatSubscriptionManager{
		client:       &http.Client{},
		authURL:      "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
		apiURL:       "https://api.access.redhat.com/management/v1/systems",
		offlineToken: offlineToken,
	}, nil
}

func (d *defaultRedHatSubscriptionManager) UnregisterInstance(machineName string) error {
	if d.credentials == nil {
		klog.Info("access token has been expired, refreshing token")
		if err := d.refreshToken(); err != nil {
			return fmt.Errorf("failed to refresh offline token")
		}
	}

	var (
		retries    = 0
		maxRetries = 15
	)

	for retries < maxRetries {
		machineUUID, err := d.findSystemsProfile(machineName)
		if err != nil {
			if err == errUnauthenticatedRequest {
				if err := d.refreshToken(); err != nil {
					klog.Errorf("failed to refresh offline token: %v", err)
					continue
				}
			}
			return fmt.Errorf("failed to find system profile: %v", err)
		}

		if machineUUID == "" {
			klog.Infof("machine uuid %s is not found", machineUUID)
			return nil
		}

		err = d.deleteSubscription(machineUUID)
		if err == nil {
			klog.Infof("subscription for vm %v has been deleted successfully", machineUUID)
			return nil
		}

		if err == errUnauthenticatedRequest {
			if err := d.refreshToken(); err != nil {
				klog.Errorf("failed to refresh offline token: %v", err)
				continue
			}
		}
		klog.Errorf("failed to delete subscription for system: %s due to: %v", machineUUID, err)
		time.Sleep(2 * time.Second)
		retries++
	}

	return errors.New("failed to delete system profile after max retires number has been reached")
}

func (d *defaultRedHatSubscriptionManager) refreshToken() error {
	payload := url.Values{}
	payload.Add("grant_type", "refresh_token")
	payload.Add("client_id", "rhsm-api")
	payload.Add("refresh_token", d.offlineToken)

	req, err := http.NewRequest("POST", d.authURL, strings.NewReader(payload.Encode()))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	var creds = &credentials{}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed while reading response: %v", err)
	}

	if res.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, creds); err != nil {
			return fmt.Errorf("failed while unmarshalling data: %v", err)
		}
		d.credentials = creds

		return nil
	}

	return fmt.Errorf("error while exeucting request with status code: %v and message: %s", res.StatusCode, string(data))
}

func (d *defaultRedHatSubscriptionManager) findSystemsProfile(name string) (string, error) {
	req, err := http.NewRequest("GET", d.apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create fecth systems request: %v", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.credentials.AccessToken))

	res, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed executing fetch systems request: %v", err)
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed while reading response: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return "", errUnauthenticatedRequest
		}
		return "", fmt.Errorf("error while exeucting request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	var fetchedSystems = &systemsResponse{}
	if err := json.Unmarshal(data, fetchedSystems); err != nil {
		return "", fmt.Errorf("failed while unmarshalling data: %v", err)
	}

	// TODO(MQ): add logic to iterate over all pages.
	for _, system := range fetchedSystems.Body {
		if system.Name == name {
			return system.UUID, nil
		}
	}

	klog.Infof("no machine name %s is found", name)
	return "", nil
}

func (d *defaultRedHatSubscriptionManager) deleteSubscription(uuid string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", d.apiURL, uuid), nil)
	if err != nil {
		return fmt.Errorf("failed to create delete system request: %v", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.credentials.AccessToken))

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

		return fmt.Errorf("error while exeucting request with status code: %v and message: %s", res.StatusCode, string(data))
	}

	return nil
}
