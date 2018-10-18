package admission

import (
	"net/http"
	"reflect"
	"time"

	"github.com/mattbaird/jsonpatch"

	"k8s.io/apimachinery/pkg/runtime"
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
	cur, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	return jsonpatch.CreatePatch(ori, cur)
}
