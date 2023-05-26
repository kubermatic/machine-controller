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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDefaultRedHatSatelliteManager_DeleteSatelliteHost(t *testing.T) {
	var (
		satelliteUsername = "satellite-user"
		satellitePassword = "satellite-password"
	)
	testCases := []struct {
		name            string
		satelliteServer string
		testingServer   *httptest.Server
	}{
		{
			name:            "execute redhat satellite unregister instance",
			satelliteServer: "satellite-test-server",
			testingServer:   createSatelliteTestingServer(satelliteUsername, satellitePassword),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				tt.testingServer.Close()
			}()

			manager := NewSatelliteSubscriptionManager()
			manager.(*DefaultSatelliteSubscriptionManager).useHTTP = true

			parsedURL, err := url.Parse(tt.testingServer.URL)
			if err != nil {
				t.Fatalf("failed to parse testing server url: %v", err)
			}

			err = manager.DeleteSatelliteHost(context.TODO(), "satellite-vm", satelliteUsername, satellitePassword, parsedURL.Host)
			if err != nil {
				t.Fatalf("failed to execute redhat host deletion")
			}
		})
	}
}

func createSatelliteTestingServer(username, password string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uname, pass, _ := r.BasicAuth()
		if uname != username || pass != password {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "fail")
		}

		if r.URL.Path != "/api/v2/hosts/satellite-vm" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "failed")
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "success")
	}))
}
