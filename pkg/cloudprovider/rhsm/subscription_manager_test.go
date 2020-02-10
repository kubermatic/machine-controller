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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	authPath = "/auth/"
	apiPath  = "/systems"
)

func TestDefaultRedHatSubscriptionManager_UnregisterInstance(t *testing.T) {
	testCases := []struct {
		name          string
		offlineToken  string
		testingServer *httptest.Server
	}{
		{
			name:          "execute redhat system unregister instance",
			offlineToken:  "test_token",
			testingServer: createTestingServer(),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				tt.testingServer.Close()
			}()
			manager, err := NewRedHatSubscriptionManager(tt.offlineToken)
			if err != nil {
				t.Fatalf("failed executing test: %v", err)
			}
			manager.(*defaultRedHatSubscriptionManager).apiURL = tt.testingServer.URL + apiPath
			manager.(*defaultRedHatSubscriptionManager).authURL = tt.testingServer.URL + authPath

			if err := manager.UnregisterInstance("test-machine-mqasim"); err != nil {
				t.Fatalf("failed executing test: %v", err)
			}
		})
	}
}

func createTestingServer() *httptest.Server {
	machineUUID := "4a3ee8d7-337d-4cef-a20c-dda011f28f95"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case authPath:
			fmt.Fprintln(w, "{\"access_token\":\"test-access-token\", \"expires_in\":900}")
		case apiPath:
			fmt.Fprintln(w, "{\"pagination\": {\"offset\": 0, \"limit\": 100,\"count\": 1}, \"body\": ["+
				"{\"name\": \"test-machine-mqasim\", \"uuid\": \""+machineUUID+"\"}]}")
		case apiPath + "/" + machineUUID:
			w.WriteHeader(http.StatusNoContent)
			fmt.Fprint(w, "success")
		}
	}))
}
