package admission

import (
	"net/http"
	"time"
)

func New(listenAddress string) *http.Server {
	m := http.NewServeMux()
	m.HandleFunc("/machinedeployments", handleFuncFactory(mutateMachineDeployments))
	return &http.Server{
		Addr:         listenAddress,
		Handler:      m,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}
