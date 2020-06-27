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
	"fmt"
	"net/http"
	"testing"
	"time"
)

var (
	satelliteUsername = "satellite-user"
	satellitePassword = "satellite-password"
)

func TestDefaultRedHatSatelliteManager_DeleteSatelliteHost(t *testing.T) {
	testCases := []struct {
		name            string
		satelliteServer string
		testingServer   *http.Server
	}{
		{
			name:            "execute redhat satellite unregister instance",
			satelliteServer: "satellite-test-server",
			testingServer:   tlsServer(),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			go tt.testingServer.ListenAndServeTLS("./certs/cert.pem", "./certs/key.pem")
			defer tt.testingServer.Close()

			time.Sleep(500 * time.Millisecond)

			manager := NewSatelliteSubscriptionManager()

			err := manager.DeleteSatelliteHost("satellite-vm", satelliteUsername, satellitePassword, tt.testingServer.Addr)
			if err != nil {
				t.Fatalf("failed to execute redhat host deletion")
			}
		})
	}
}

func tlsServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uname, pass, _ := r.BasicAuth()
		if uname != satelliteUsername || pass != satellitePassword {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "fail")
		}

		if r.URL.Path != "/api/hosts/satellite-vm" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "failed")
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "success")
	}))

	return &http.Server{
		Addr:    ":3000",
		Handler: mux,
	}
}
