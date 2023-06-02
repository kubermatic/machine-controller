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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	authPath = "/"
	apiPath  = "/systems"
)

func TestDefaultRedHatSubscriptionManager_UnregisterInstance(t *testing.T) {
	testCases := []struct {
		name           string
		requestLimiter int
		offlineToken   string
		testingServer  *httptest.Server
		machineName    string
	}{
		{
			name:           "execute redhat system unregister instance",
			requestLimiter: 2,
			offlineToken:   "test_token",
			testingServer:  createTestingServer(false),
			machineName:    "test-machine",
		},
		{
			name:           "execute redhat system unregister instance for 5 systems",
			requestLimiter: 2,
			offlineToken:   "test_token",
			testingServer:  createTestingServer(true),
			machineName:    "test-machine-5",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				tt.testingServer.Close()
			}()
			manager := NewRedHatSubscriptionManager()
			manager.(*defaultRedHatSubscriptionManager).apiURL = tt.testingServer.URL + apiPath
			manager.(*defaultRedHatSubscriptionManager).authURL = tt.testingServer.URL
			manager.(*defaultRedHatSubscriptionManager).requestsLimiter = tt.requestLimiter

			if err := manager.UnregisterInstance(context.Background(), tt.offlineToken, tt.machineName); err != nil {
				t.Fatalf("failed executing test: %v", err)
			}
		})
	}
}

func createTestingServer(pagination bool) *httptest.Server {
	var (
		processedRequest = 1
		result           string
	)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case authPath:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, "{\"access_token\":\"test-access-token\"}")
		case apiPath:
			if pagination {
				switch processedRequest {
				case 1:
					result = "{\"pagination\": {\"offset\": 0, \"limit\": 2,\"count\": 5}, \"body\": [" +
						"{\"name\": \"test-machine-1\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f96\"}," +
						"{\"name\": \"test-machine-2\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f91\"}" +
						"]}"
				case 2:
					result = "{\"pagination\": {\"offset\": 0, \"limit\": 2,\"count\": 5}, \"body\": [" +
						"{\"name\": \"test-machine-3\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f98\"}," +
						"{\"name\": \"test-machine-4\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f95\"}" +
						"]}"
				case 3:
					result = "{\"pagination\": {\"offset\": 0, \"limit\": 2,\"count\": 5}, \"body\": [" +
						"{\"name\": \"test-machine-5\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f99\"}" +
						"]}"
				}
				processedRequest++
				fmt.Fprint(w, result)
			} else {
				processedRequest++
				fmt.Fprintln(w, "{\"pagination\": {\"offset\": 0, \"limit\": 100,\"count\": 1}, \"body\": ["+
					"{\"name\": \"test-machine\", \"uuid\": \"4a3ee8d7-337d-4cef-a20c-dda011f28f95\"}]}")
			}
		case apiPath + "/4a3ee8d7-337d-4cef-a20c-dda011f28f95":
			w.WriteHeader(http.StatusNoContent)
			fmt.Fprint(w, "success")
		case apiPath + "/4a3ee8d7-337d-4cef-a20c-dda011f28f99":
			w.WriteHeader(http.StatusNoContent)
			fmt.Fprint(w, "success")
		}
	}))
}
