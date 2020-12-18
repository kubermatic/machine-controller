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
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"github.com/mattbaird/jsonpatch"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"
)

type admissionData struct {
	client          ctrlruntimeclient.Client
	userDataManager *userdatamanager.Manager
}

var jsonPatch = admissionv1beta1.PatchTypeJSONPatch

func New(listenAddress string, client ctrlruntimeclient.Client, um *userdatamanager.Manager) *http.Server {
	m := http.NewServeMux()
	ad := &admissionData{
		client:          client,
		userDataManager: um,
	}
	m.HandleFunc("/machinedeployments", handleFuncFactory(ad.mutateMachineDeployments))
	m.HandleFunc("/machines", handleFuncFactory(ad.mutateMachines))
	m.HandleFunc("/healthz", healthZHandler)

	return &http.Server{
		Addr:    listenAddress,
		Handler: m,
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
	klog.V(6).Infof("jsonpatch: Marshaled original: %s", string(ori))
	cur, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	klog.V(6).Infof("jsonpatch: Marshaled target: %s", string(cur))
	return jsonpatch.CreatePatch(ori, cur)
}

func createAdmissionResponse(original, mutated runtime.Object) (*admissionv1beta1.AdmissionResponse, error) {
	response := &admissionv1beta1.AdmissionResponse{}
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

type mutator func(context.Context, admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error)

// handleFuncFactory wraps a mutator as an HTTP HandlerFunc.
func handleFuncFactory(mutate mutator) func(http.ResponseWriter, *http.Request) {
	/*
		Within the handler we create below, the actual mutation logic is
		running in a separate goroutine. This is because sometimes some
		cloud provider APIs are slow, very slow, and the kube-apiserver is
		impatient and will wait at most 30 seconds (10 by default) for a
		response.

		To ensure faster responses, we use our CloudproviderCache wrapper,
		which will cache the results based on the MachineSpec's JSON
		representation. While this just generally speeds up responses, this
		becomes *critical* for slow cloud providers. For those, it can happen
		that no validation ever succeeds within the given timeout. To ensure
		that these providers can be used at all, we must let the validation run
		in the background, cache its result and inform the user upon a timeout
		that the validation is still happening and they should try again later,
		when we have cached the result.

		We plan the timing below based on a 10s timeout, as this is much less
		painful for users that would have to wait up to 30s for any kind of
		feedback for their `kubectl apply` command.
	*/
	return func(w http.ResponseWriter, r *http.Request) {
		// this context is used for the validation logic
		validationCtx := context.Background()

		// kube-apiserver waits for 10s by default, so we give us 9s time
		waitCtx, waitCancel := context.WithTimeout(validationCtx, 9*time.Second)
		defer waitCancel()

		// this is where we receive the validation result or error
		responses := make(chan *admissionv1beta1.AdmissionResponse, 1)
		errs := make(chan error, 1)

		// start the validation
		go func() {
			response, err := admissionExecutor(validationCtx, r, mutate)
			if err != nil {
				errs <- err
			} else {
				responses <- response
			}

			close(responses)
			close(errs)
		}()

		var response *admissionv1beta1.AdmissionResponse

		// wait for either a result to appear or the timeout kicking in
		select {
		// happy path: validation completed
		case response = <-responses:
			// nop

		// validation errored
		case err := <-errs:
			response = &admissionv1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}

		// give up and at least produce an informative error message for the user
		case <-waitCtx.Done():
			klog.Warning("failed to complete admission request within deadline; processing continues in the background, but the request has to be retried")

			response = &admissionv1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: "validation timed out, please try again later",
					Code:    http.StatusGatewayTimeout,
				},
			}

		// request has been cancelled
		case <-r.Context().Done():
			klog.Errorf("failed to complete admission request within deadline and validation request has been cancelled already")
		}

		// if we have a response, send it out; otherwise the request has
		// already been cancelled and writing a response is futile
		if response != nil {
			admissionReview := admissionv1beta1.AdmissionReview{
				Response: response,
			}

			resp, err := json.Marshal(admissionReview)
			if err != nil {
				klog.Errorf("failed to marshal admissionReview: %v", err)
				return
			}

			if _, err := w.Write(resp); err != nil {
				klog.Errorf("failed to write admissionReview: %v", err)
			}
		}
	}
}

func admissionExecutor(ctx context.Context, r *http.Request, mutate mutator) (*admissionv1beta1.AdmissionResponse, error) {
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

	admissionReview := admissionv1beta1.AdmissionReview{}
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request into admissionReview: %v", err)
	}

	admissionResponse, err := mutate(ctx, admissionReview)
	if err != nil {
		return nil, fmt.Errorf("defaulting or validation failed: %v", err)
	}

	return admissionResponse, nil
}
