package admission

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var codecs = serializer.NewCodecFactory(runtime.NewScheme())

func mutateMachineDeployments(ar admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {

	machineDeployment := &clusterv1alpha1.MachineDeployment{}
	if err := json.Unmarshal(ar.Request.Object.Raw, machineDeployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %v", err)
	}

	return nil, fmt.Errorf("unimplemented")
}

func handleFuncFactory(mutate func(admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			if data, err := ioutil.ReadAll(r.Body); err == nil {
				body = data
			}
		}

		// verify the content type is accurate
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			glog.Errorf("contentType=%s, expect application/json", contentType)
			return
		}

		var reviewResponse *admissionv1beta1.AdmissionResponse
		ar := admissionv1beta1.AdmissionReview{}
		deserializer := codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
			glog.Error(err)
			reviewResponse.Result = &metav1.Status{Message: err.Error()}
		} else {
			reviewResponse, err = mutate(ar)
			if err != nil {
				glog.Errorf("Error mutating %v", err)
			}
		}

		response := admissionv1beta1.AdmissionReview{}
		if reviewResponse != nil {
			response.Response = reviewResponse
			response.Response.UID = ar.Request.UID
		} else {
			// Required to not have the apiserver crash with an NPE on older versions
			// https://github.com/kubernetes/apiserver/commit/584fe98b6432033007b686f1b8063e05d20d328d
			response.Response = &admissionv1beta1.AdmissionResponse{}
		}

		// reset the Object and OldObject, they are not needed in a response.
		ar.Request.Object = runtime.RawExtension{}
		ar.Request.OldObject = runtime.RawExtension{}

		resp, err := json.Marshal(response)
		if err != nil {
			glog.Errorf("failed to marshal response: %v", err)
			return
		}
		if _, err := w.Write(resp); err != nil {
			glog.Errorf("failed to write response: %v", err)
		}
	}
}
