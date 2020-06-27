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
