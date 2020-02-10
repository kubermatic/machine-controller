package rhsm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	authPath    = "/auth/"
	apiPath     = "/systems"
	machineUUID = "4a3ee8d7-337d-4cef-a20c-dda011f28f95"
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
			manager.(*defaultRedHatSubscriptionManager).apiUrl = tt.testingServer.URL + apiPath
			manager.(*defaultRedHatSubscriptionManager).authUrl = tt.testingServer.URL + authPath

			if err := manager.UnregisterInstance("test-machine-mqasim"); err != nil {
				t.Fatalf("failed executing test: %v", err)
			}
		})
	}
}

func createTestingServer() *httptest.Server {
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
