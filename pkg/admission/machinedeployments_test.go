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
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"go.uber.org/zap/zaptest"

	"k8c.io/machine-controller/pkg/cloudprovider/provider/fake"
	machinecontroller "k8c.io/machine-controller/pkg/controller/machine"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	"k8c.io/machine-controller/sdk/providerconfig"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMachineDeploymentDefaulting(t *testing.T) {
	tests := []struct {
		name              string
		machineDeployment *clusterv1alpha1.MachineDeployment
		isValid           bool
	}{
		{
			name:              "Empty MachineDeployment validation should fail",
			machineDeployment: &clusterv1alpha1.MachineDeployment{},
			isValid:           false,
		},
		{
			name: "Minimal MachineDeployment validation should succeed",
			machineDeployment: &clusterv1alpha1.MachineDeployment{
				Spec: clusterv1alpha1.MachineDeploymentSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: clusterv1alpha1.MachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"foo": "bar"},
						},
					},
				},
			},
			isValid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machineDeploymentDefaultingFunction(test.machineDeployment)
			errs := validateMachineDeployment(*test.machineDeployment)
			if test.isValid != (len(errs) == 0) {
				t.Errorf("Expected machine to be valid: %t but got %d errors: %v", test.isValid, len(errs), errs)
			}
		})
	}
}

func newTestAdmissionData(t *testing.T) *admissionData {
	t.Helper()
	c, err := semver.NewConstraint(">= 1.0.0")
	if err != nil {
		t.Fatalf("constraint: %v", err)
	}
	return &admissionData{
		log:          zaptest.NewLogger(t).Sugar(),
		client:       clientfake.NewClientBuilder().Build(),
		workerClient: clientfake.NewClientBuilder().Build(),
		nodeSettings: machinecontroller.NodeSettings{},
		namespace:    "kube-system",
		constraints:  c,
	}
}

func newAdmissionRequest(t *testing.T, op admissionv1.Operation, newMD, oldMD *clusterv1alpha1.MachineDeployment) admissionv1.AdmissionRequest {
	t.Helper()
	objRaw, err := json.Marshal(newMD)
	if err != nil {
		t.Fatalf("marshal newMD: %v", err)
	}
	req := admissionv1.AdmissionRequest{
		UID:       "test-uid",
		Operation: op,
		Object:    runtime.RawExtension{Raw: objRaw},
	}
	if oldMD != nil {
		oldRaw, err := json.Marshal(oldMD)
		if err != nil {
			t.Fatalf("marshal oldMD: %v", err)
		}
		req.OldObject = runtime.RawExtension{Raw: oldRaw}
	}
	return req
}

type mdBuilder struct {
	deletionTimestamp *metav1.Time
	passValidation    bool
	kubelet           string
	image             string
	noProviderSpec    bool
}

func mdBuild() *mdBuilder {
	return &mdBuilder{passValidation: true, kubelet: "1.30.0", image: "img-default"}
}

func (b *mdBuilder) withDeletionTimestamp(ts metav1.Time) *mdBuilder {
	b.deletionTimestamp = &ts
	return b
}
func (b *mdBuilder) withPassValidation(v bool) *mdBuilder   { b.passValidation = v; return b }
func (b *mdBuilder) withKubeletVersion(v string) *mdBuilder { b.kubelet = v; return b }
func (b *mdBuilder) withImage(s string) *mdBuilder          { b.image = s; return b }
func (b *mdBuilder) withNoProviderSpec() *mdBuilder         { b.noProviderSpec = true; return b }

func (b *mdBuilder) build(t *testing.T) *clusterv1alpha1.MachineDeployment {
	t.Helper()

	md := &clusterv1alpha1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "md", Namespace: "kube-system"},
		Spec: clusterv1alpha1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			Template: clusterv1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
				Spec: clusterv1alpha1.MachineSpec{
					Versions: clusterv1alpha1.MachineVersionInfo{Kubelet: b.kubelet},
				},
			},
		},
	}

	if b.deletionTimestamp != nil {
		md.DeletionTimestamp = b.deletionTimestamp
	}

	if b.noProviderSpec {
		return md
	}

	fakeSpec := fake.CloudProviderSpec{PassValidation: b.passValidation}
	fakeRaw, err := json.Marshal(&fakeSpec)
	if err != nil {
		t.Fatalf("marshal fake spec: %v", err)
	}
	cfg := providerconfig.Config{
		CloudProvider:     providerconfig.CloudProviderFake,
		CloudProviderSpec: runtime.RawExtension{Raw: fakeRaw},
		OperatingSystem:   providerconfig.OperatingSystemUbuntu,
	}
	rawCfg, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal providerconfig: %v", err)
	}
	md.Spec.Template.Spec.ProviderSpec = clusterv1alpha1.ProviderSpec{
		Value: &runtime.RawExtension{Raw: rawCfg},
	}
	return md
}

func TestMutateMachineDeployments(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()

	tests := []struct {
		name              string
		op                admissionv1.Operation
		newMD             *clusterv1alpha1.MachineDeployment
		oldMD             *clusterv1alpha1.MachineDeployment
		wantAllowed       bool
		wantErrorContains string
	}{
		{
			name:        "create_with_resolvable_image",
			op:          admissionv1.Create,
			newMD:       mdBuild().build(t),
			wantAllowed: true,
		},
		{
			name:              "create_with_unresolvable_image_rejected",
			op:                admissionv1.Create,
			newMD:             mdBuild().withPassValidation(false).build(t),
			wantErrorContains: "failing validation as requested",
		},
		{
			name:              "create_with_empty_kubelet_rejected",
			op:                admissionv1.Create,
			newMD:             mdBuild().withKubeletVersion("").build(t),
			wantErrorContains: "kubelet version must be set",
		},
		{
			name:        "update_label_only_with_resolvable_image",
			op:          admissionv1.Update,
			newMD:       mdBuild().build(t),
			oldMD:       mdBuild().build(t),
			wantAllowed: true,
		},
		{
			name:              "update_changing_image_to_unresolvable_rejected",
			op:                admissionv1.Update,
			newMD:             mdBuild().withPassValidation(false).withImage("new-img").build(t),
			oldMD:             mdBuild().build(t),
			wantErrorContains: "failing validation as requested",
		},
		{
			name:        "delete_md_with_resolvable_image_succeeds",
			op:          admissionv1.Update,
			newMD:       mdBuild().withDeletionTimestamp(now).build(t),
			oldMD:       mdBuild().withDeletionTimestamp(now).build(t),
			wantAllowed: true,
		},
		{
			// https://github.com/kubermatic/kubermatic/issues/15802
			// oldMD has passValidation=true to ensure the post-mutation provider
			// config bytes differ from the new MD (passValidation=false), forcing
			// the webhook to run Validate() on the deleting object.
			name:        "delete_md_with_unresolvable_image_succeeds",
			op:          admissionv1.Update,
			newMD:       mdBuild().withDeletionTimestamp(now).withPassValidation(false).build(t),
			oldMD:       mdBuild().withDeletionTimestamp(now).withPassValidation(true).build(t),
			wantAllowed: true,
		},
		{
			name:        "delete_md_with_unresolvable_image_and_empty_kubelet_succeeds",
			op:          admissionv1.Update,
			newMD:       mdBuild().withDeletionTimestamp(now).withPassValidation(false).withKubeletVersion("").build(t),
			oldMD:       mdBuild().withDeletionTimestamp(now).withPassValidation(true).withKubeletVersion("").build(t),
			wantAllowed: true,
		},
		{
			name:        "delete_md_with_no_provider_spec_succeeds",
			op:          admissionv1.Update,
			newMD:       mdBuild().withDeletionTimestamp(now).withNoProviderSpec().build(t),
			oldMD:       mdBuild().withDeletionTimestamp(now).withNoProviderSpec().build(t),
			wantAllowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ad := newTestAdmissionData(t)
			req := newAdmissionRequest(t, tc.op, tc.newMD, tc.oldMD)
			resp, err := ad.mutateMachineDeployments(ctx, req)

			if tc.wantErrorContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrorContains)
				}
				if !strings.Contains(err.Error(), tc.wantErrorContains) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if resp == nil {
				t.Fatal("expected non-nil response")
			}

			if resp.Allowed != tc.wantAllowed {
				t.Fatalf("Allowed: want %v, got %v", tc.wantAllowed, resp.Allowed)
			}
		})
	}
}
