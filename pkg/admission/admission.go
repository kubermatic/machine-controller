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

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"gomodules.xyz/jsonpatch/v2"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	"github.com/kubermatic/machine-controller/pkg/node"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type admissionData struct {
	client          ctrlruntimeclient.Client
	workerClient    ctrlruntimeclient.Client
	userDataManager *userdatamanager.Manager
	nodeSettings    machinecontroller.NodeSettings
	useOSM          bool
	namespace       string
}

var jsonPatch = admissionv1.PatchTypeJSONPatch

func New(
	listenAddress string,
	client ctrlruntimeclient.Client,
	workerClient ctrlruntimeclient.Client,
	um *userdatamanager.Manager,
	nodeFlags *node.Flags,
	useOSM bool,
	namespace string,
) (*http.Server, error) {
	mux := http.NewServeMux()
	ad := &admissionData{
		client:          client,
		workerClient:    workerClient,
		userDataManager: um,
		useOSM:          useOSM,
		namespace:       namespace,
	}

	if err := nodeFlags.UpdateNodeSettings(&ad.nodeSettings); err != nil {
		return nil, fmt.Errorf("error updating nodeSettings, %w", err)
	}

	mux.HandleFunc("/machinedeployments", handleFuncFactory(ad.mutateMachineDeployments))
	mux.HandleFunc("/machines", handleFuncFactory(ad.mutateMachines))
	mux.HandleFunc("/healthz", healthZHandler)

	return &http.Server{
		Addr:    listenAddress,
		Handler: http.TimeoutHandler(mux, 25*time.Second, "timeout"),
	}, nil
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
	klog.V(6).Infof("jsonpatch: Marshaled original: %s", string(ori))
	cur, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	klog.V(6).Infof("jsonpatch: Marshaled target: %s", string(cur))
	return jsonpatch.CreatePatch(ori, cur)
}

func createAdmissionResponse(original, mutated runtime.Object) (*admissionv1.AdmissionResponse, error) {
	response := &admissionv1.AdmissionResponse{}
	response.Allowed = true
	if !apiequality.Semantic.DeepEqual(original, mutated) {
		patchOpts, err := newJSONPatch(original, mutated)
		if err != nil {
			return nil, fmt.Errorf("failed to create json patch: %v", err)
		}

		patchRaw, err := json.Marshal(patchOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json patch: %v", err)
		}
		klog.V(3).Infof("Produced jsonpatch: %s", string(patchRaw))

		response.Patch = patchRaw
		response.PatchType = &jsonPatch
	}
	return response, nil
}

type mutator func(context.Context, admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error)

func handleFuncFactory(mutate mutator) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		review, err := readReview(r)
		if err != nil {
			klog.Warningf("invalid admission review: %v", err)

			// proper AdmissionReview responses require metadata that is not available
			// in broken requests, so we return a basic failure response
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(fmt.Sprintf("invalid request: %v", err))); err != nil {
				klog.Errorf("failed to write badRequest: %v", err)
			}
			return
		}

		// run the mutation logic
		response, err := mutate(r.Context(), *review.Request)
		if err != nil {
			response = &admissionv1.AdmissionResponse{}
			response.Result = &metav1.Status{Message: err.Error()}
		}
		response.UID = review.Request.UID

		resp, err := json.Marshal(&admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				APIVersion: admissionv1.SchemeGroupVersion.String(),
				Kind:       "AdmissionReview",
			},
			Response: response,
		})
		if err != nil {
			klog.Errorf("failed to marshal admissionResponse: %v", err)
			return
		}

		if _, err := w.Write(resp); err != nil {
			klog.Errorf("failed to write admissionResponse: %v", err)
		}
	}
}

func readReview(r *http.Request) (*admissionv1.AdmissionReview, error) {
	var body []byte
	if r.Body == nil {
		return nil, fmt.Errorf("request has no body")
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading data from request body: %v", err)
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		return nil, fmt.Errorf("header Content-Type was %s, expected application/json", contentType)
	}

	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request into admissionReview: %v", err)
	}
	if admissionReview.Request == nil {
		return nil, errors.New("invalid admission review: no request defined")
	}

	return admissionReview, nil
}
