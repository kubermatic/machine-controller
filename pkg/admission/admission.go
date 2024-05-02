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
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"go.uber.org/zap"
	"gomodules.xyz/jsonpatch/v2"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	"github.com/kubermatic/machine-controller/pkg/node"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type admissionData struct {
	log          *zap.SugaredLogger
	client       ctrlruntimeclient.Client
	workerClient ctrlruntimeclient.Client
	nodeSettings machinecontroller.NodeSettings
	namespace    string
	constraints  *semver.Constraints
}

var jsonPatch = admissionv1.PatchTypeJSONPatch

type Builder struct {
	ListenAddress      string
	Log                *zap.SugaredLogger
	Client             ctrlruntimeclient.Client
	WorkerClient       ctrlruntimeclient.Client
	NodeFlags          *node.Flags
	Namespace          string
	VersionConstraints *semver.Constraints

	CertDir  string
	CertName string
	KeyName  string
}

func (build Builder) Build() (webhook.Server, error) {
	ad := &admissionData{
		log:          build.Log,
		client:       build.Client,
		workerClient: build.WorkerClient,
		namespace:    build.Namespace,
		constraints:  build.VersionConstraints,
	}

	if err := build.NodeFlags.UpdateNodeSettings(&ad.nodeSettings); err != nil {
		return nil, fmt.Errorf("error updating nodeSettings, %w", err)
	}

	options := webhook.Options{
		CertDir:  build.CertDir,
		CertName: build.CertName,
		KeyName:  build.KeyName,
	}

	if build.ListenAddress != "" {
		host, port, err := net.SplitHostPort(build.ListenAddress)
		if err != nil {
			return nil, fmt.Errorf("error parsing ListenAddress: %w", err)
		}

		options.Host = host

		if port != "" {
			port, err := strconv.ParseInt(port, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("error parsing port from ListenAddress: %w", err)
			}

			options.Port = int(port)
		}
	}

	server := webhook.NewServer(options)

	server.Register("/machinedeployments", handleFuncFactory(build.Log, ad.mutateMachineDeployments))
	server.Register("/machines", handleFuncFactory(build.Log, ad.mutateMachines))

	checkers := healthz.Handler{
		Checks: map[string]healthz.Checker{
			"ping": healthz.Ping,
		},
	}
	server.Register("/healthz/", http.StripPrefix("/healthz/", &checkers))

	return server, nil
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

func createAdmissionResponse(log *zap.SugaredLogger, original, mutated runtime.Object) (*admissionv1.AdmissionResponse, error) {
	response := &admissionv1.AdmissionResponse{}
	response.Allowed = true
	if !apiequality.Semantic.DeepEqual(original, mutated) {
		patchOpts, err := newJSONPatch(original, mutated)
		if err != nil {
			return nil, fmt.Errorf("failed to create json patch: %w", err)
		}

		patchRaw, err := json.Marshal(patchOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json patch: %w", err)
		}
		log.Debugw("Produced jsonpatch", "patch", string(patchRaw))

		response.Patch = patchRaw
		response.PatchType = &jsonPatch
	}
	return response, nil
}

type mutator func(context.Context, admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error)

func handleFuncFactory(log *zap.SugaredLogger, mutate mutator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		review, err := readReview(r)
		if err != nil {
			log.Errorw("Invalid admission review", zap.Error(err))

			// proper AdmissionReview responses require metadata that is not available
			// in broken requests, so we return a basic failure response
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(fmt.Sprintf("invalid request: %v", err))); err != nil {
				log.Errorw("Failed to write badRequest", zap.Error(err))
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
			log.Errorw("Failed to marshal admissionResponse", zap.Error(err))
			return
		}

		if _, err := w.Write(resp); err != nil {
			log.Errorw("Failed to write admissionResponse", zap.Error(err))
		}
	}
}

func readReview(r *http.Request) (*admissionv1.AdmissionReview, error) {
	var body []byte
	if r.Body == nil {
		return nil, fmt.Errorf("request has no body")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading data from request body: %w", err)
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		return nil, fmt.Errorf("header Content-Type was %s, expected application/json", contentType)
	}

	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request into admissionReview: %w", err)
	}
	if admissionReview.Request == nil {
		return nil, errors.New("invalid admission review: no request defined")
	}

	return admissionReview, nil
}
