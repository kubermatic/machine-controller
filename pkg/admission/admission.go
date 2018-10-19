package admission

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/mattbaird/jsonpatch"

	"k8s.io/apimachinery/pkg/runtime"
)

func New(listenAddress string) *http.Server {
	m := http.NewServeMux()
	m.HandleFunc("/machinedeployments", handleFuncFactory(mutateMachineDeployments))
	m.HandleFunc("/healthz", healthZHandler)
	return &http.Server{
		Addr:         listenAddress,
		Handler:      m,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

func healthZHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func newJSONPatch(original, current runtime.Object) ([]jsonpatch.JsonPatchOperation, error) {
	originalGVK := original.GetObjectKind().GroupVersionKind()
	currentGVK := current.GetObjectKind().GroupVersionKind()
	if !reflect.DeepEqual(originalGVK, currentGVK) {
		return nil, fmt.Errorf("GroupVersionKind %#v is expected to match %#v", originalGVK, currentGVK)
	}
	ori, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("jsonpatch: Marshaled original: %s", string(ori))
	cur, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("jsonpatch: Marshaled target: %s", string(cur))
	return jsonpatch.CreatePatch(ori, cur)
}
