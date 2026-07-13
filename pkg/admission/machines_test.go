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
	"reflect"
	"strings"
	"testing"

	"k8c.io/machine-controller/pkg/cloudprovider/provider/fake"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	"k8c.io/machine-controller/sdk/providerconfig"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	validRSA1024Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCLFEu0y7Gl2sG0TCHKBKntvzf5Dszt/SWm5GJXIriGCAKdaOKqmeA/AfECqkE9q/omX8rkr+4RdLVRm2ybkQHYinf7IUmmWcjifnB2STDVeHBkgggYY0MC0Dom5pYMfklUZSWiH1XulFSZd7XsCKcxIloWxxljunsv2BUhUaguSw==`
	validRSA2048Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDk6xzVo5JU9FYzE6HNZeMq9mqNfMOr6rBX7QZ317ZL1TMSdIvvQzvuJmn0ZvkrwpT5vLsSYQex9gz/62xr6Unb7i7rXUsPhq4TDDucWwGis7GJ78lFvt4kPW81kqPJiiSh3uIUA/enVLBrXZbGLd1AfHd+rENrhjq6mFyd42CbNunHPiQAgMJKZ3mRb/llzo5fKZeR1KbETwsjVbPkD5fW026HlIsT8QJ49ya7xuZCgF9iPcL9EUTpQkK60r4iNAnzodlS5YsErLck+P+Jw1xEJ+hw0BTBgXtFQznTVFMrV7E408o9+UY/t7Sb6wE1HUEDbIdaKyPUT158FNugVeP7`
	validRSA4096Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCSskDVXwtW3fVkpbZkCeWj/aBr8lE+NyHbgVsAbmylb1MzrjWAJ1ynFSRCFk1fBql1zrmNJPeT6d5SkfvqExRiaC7KZ+oucvM9lkjyVwREQEF1d5iBQr3268C+S4HKgKxFYaJQwMYw7bYnE7np6kHwTOTX5sOC4imFWKR40X385yItbkmL35ZgIJQB8/W+TCEU4wEND5Kf3m85d1pIVCTqn3NTf3s3BtezK1AJQtzqJDVGALqrmgf9+fGM8yheMRKKwVi378hkREI4oOcWppLV20IDKE4OFm6ZW+U414zcq75WkebRgThK4Y0EepqUxebd1A3KoTeEMaJeHGUmhq6YOjKkAg9PyTKNBsDjwwOCzIuoFbmEOq7H9e3fE670unuM/O92NOwPK7XTedNryNs7QMe+UPzO3HP9nGYziy+rBCgnGs2QJjYya8ReKKB34G9VtBRn7vRd0lXliVFjUcQKhpClJdVENVbRH3MJrsE+iWOf46u8kI9xrSAdo0BX6z0x/ujIH8cI1FFZZxToSWP/VrqIr0wtMOwiQ7j5VeEFN1S4ACYm/dzzG01j0Xr0bdJ/2PqSASf4S1HEI9KEzLWYIHtjhHjLwS8narweW0fMjtUu2tRlwxGoS5aZP4JYIjHd9DWlkczswDkh2OmMpaNPXnr3f2BITxgea/SPkoUdYw==`

	validECDSA256Key = `ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFDpHqvvX0D9iLccVi53JthsQ4xSJicRfl0oAPCdTgyGAQG1RXI435o4wG+bAD1zOUMjdfd2iVykwRZ6R7+yOaM=`
	validECDSA384Key = `ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBFm76lHa5m5O1nOyQTyG6JoSktq6/l3tHj80nuxfxV7xJwV47guLwFsK5vGpnhFcC3cmBl2GO/deis6EalCaaWoi/sGEnJrFCLUEMxRojX/pHNYPaU4R2DZnj0Y2w/y03A==`
	validECDSA521Key = `ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAG2GjQxcul7nGRsuEtbTfKxYskKaYMaGPLwptG+1PQRPDGgTEqiGUToDYXhui6DyGKIZz/3i6iYyeEVgz6+wc+eqAENZMrA7qi0t4NXk6ky56PJeLLHb9Ry0Isdi6idoIZnKrv184Afc4hY4EeiK8Q+oseQwjLYg+0gwb+q9zPr4IXuZA==`

	validDSA1024Key = `ssh-dss AAAAB3NzaC1kc3MAAACBAP1/U4EddRIpUt9KnC7s5Of2EbdSPO9EAMMeP4C2USZpRV1AIlH7WT2NWPq/xfW6MPbLm1Vs14E7gB00b/JmYLdrmVClpJ+f6AR7ECLCT7up1/63xhv4O1fnxqimFQ8E+4P208UewwI1VBNaFpEy9nXzrith1yrv8iIDGZ3RSAHHAAAAFQCXYFCPFSMLzLKSuYKi64QL8Fgc9QAAAIEA9+GghdabPd7LvKtcNrhXuXmUr7v6OuqC+VdMCz0HgmdRWVeOutRZT+ZxBxCBgLRJFnEj6EwoFhO3zwkyjMim4TwWeotUfI0o4KOuHiuzpnWRbqN/C/ohNWLx+2J6ASQ7zKTxvqhRkImog9/hWuWfBpKLZl6Ae1UlZAFMO/7PSSoAAACAU5qGNxrBT4VDW1bN1m6szPH4PRlqNSPHNG/1Xs3LrJyGRXxnl218IYyrfAb+lIIEZEcUFGGWyRJOLQhmWv68zBupKv1JJaVAQ4JTMPPmmPwGus01eSGd9NjAS6Qtl9FGMLrLFk4IRFuenHWOas1PzDlOXybUnaXtQpNcKEJgMik=`
)

// TestDefaultAndValidateMachineSpecPersistsProviderDefaults ensures provider
// AddDefaults results reach the caller's spec, since mutateMachines builds the
// admission patch from that object, not from the returned copy. uses the
// hetzner datacenter -> location migration as the observable default.
func TestDefaultAndValidateMachineSpecPersistsProviderDefaults(t *testing.T) {
	// no token keeps the hetzner provider offline: Validate fails with "token
	// is missing" before any API call, after AddDefaults already ran and its
	// result was written back.
	t.Setenv("HZ_TOKEN", "")

	// raw JSON to avoid referencing the deprecated Datacenter field outside
	// the hetzner package.
	spec := machineSpecWithProviderConfig(t, providerconfig.CloudProviderHetzner, []byte(`{"serverType":"cx22","datacenter":"nbg1-dc3"}`))

	ad := newTestAdmissionData(t)
	err := ad.defaultAndValidateMachineSpec(context.Background(), &spec)
	if err == nil || !strings.Contains(err.Error(), "token is missing") {
		t.Fatalf("defaultAndValidateMachineSpec() error = %v, want the offline \"token is missing\" validation error", err)
	}

	defaultedPconfig, err := providerconfig.GetConfig(spec.ProviderSpec)
	if err != nil {
		t.Fatalf("failed to read providerconfig back from spec: %v", err)
	}
	cloudProviderSpec := map[string]any{}
	if err := json.Unmarshal(defaultedPconfig.CloudProviderSpec.Raw, &cloudProviderSpec); err != nil {
		t.Fatalf("failed to unmarshal cloudProviderSpec: %v", err)
	}

	if dc := cloudProviderSpec["datacenter"]; dc != "" {
		t.Errorf("caller-visible datacenter = %v, want empty; AddDefaults result was lost", dc)
	}
	if loc := cloudProviderSpec["location"]; loc != "nbg1" {
		t.Errorf("caller-visible location = %v, want %q; AddDefaults result was lost", loc, "nbg1")
	}
}

func machineSpecWithProviderConfig(t *testing.T, cloudProvider providerconfig.CloudProvider, cloudProviderSpec []byte) clusterv1alpha1.MachineSpec {
	t.Helper()

	pconfig := providerconfig.Config{
		CloudProvider:       cloudProvider,
		CloudProviderSpec:   runtime.RawExtension{Raw: cloudProviderSpec},
		OperatingSystem:     providerconfig.OperatingSystemUbuntu,
		OperatingSystemSpec: runtime.RawExtension{Raw: []byte("{}")},
	}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		t.Fatalf("failed to marshal providerconfig: %v", err)
	}

	return clusterv1alpha1.MachineSpec{
		ProviderSpec: clusterv1alpha1.ProviderSpec{Value: &runtime.RawExtension{Raw: rawPconfig}},
		Versions:     clusterv1alpha1.MachineVersionInfo{Kubelet: "1.30.0"},
	}
}

// TestDefaultAndValidateMachineSpecPreservesSpecFields guards the write-back of
// the AddDefaults result (*spec = defaultedSpec): for providers that do not
// default anything, the round-trip through the by-value copy must not lose or
// alter any field of the caller's spec.
func TestDefaultAndValidateMachineSpecPreservesSpecFields(t *testing.T) {
	fakeSpec, err := json.Marshal(fake.CloudProviderSpec{PassValidation: true})
	if err != nil {
		t.Fatalf("failed to marshal fake cloud provider spec: %v", err)
	}

	spec := machineSpecWithProviderConfig(t, providerconfig.CloudProviderFake, fakeSpec)
	spec.Name = "worker-0"
	spec.Labels = map[string]string{"role": "worker"}
	spec.Annotations = map[string]string{"team": "dev-kkp"}
	spec.Taints = []corev1.Taint{{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule}}

	want := *spec.DeepCopy()

	ad := newTestAdmissionData(t)
	if err := ad.defaultAndValidateMachineSpec(context.Background(), &spec); err != nil {
		t.Fatalf("defaultAndValidateMachineSpec() error = %v", err)
	}

	if spec.Name != want.Name {
		t.Errorf("name = %q, want %q", spec.Name, want.Name)
	}
	if !reflect.DeepEqual(spec.Labels, want.Labels) {
		t.Errorf("labels = %v, want %v", spec.Labels, want.Labels)
	}
	if !reflect.DeepEqual(spec.Annotations, want.Annotations) {
		t.Errorf("annotations = %v, want %v", spec.Annotations, want.Annotations)
	}
	if !reflect.DeepEqual(spec.Taints, want.Taints) {
		t.Errorf("taints = %v, want %v", spec.Taints, want.Taints)
	}
	if !reflect.DeepEqual(spec.Versions, want.Versions) {
		t.Errorf("versions = %v, want %v", spec.Versions, want.Versions)
	}

	// the provider spec is re-marshaled for OS defaulting, but must still parse
	// and keep the cloud provider intact.
	pconfig, err := providerconfig.GetConfig(spec.ProviderSpec)
	if err != nil {
		t.Fatalf("failed to read providerconfig back from spec: %v", err)
	}
	if pconfig.CloudProvider != providerconfig.CloudProviderFake {
		t.Errorf("cloudProvider = %q, want %q", pconfig.CloudProvider, providerconfig.CloudProviderFake)
	}
}

// TestDefaultAndValidateMachineSpecKubeVirt covers the KubeVirt call-site
// contract: the annotations map is initialized on the caller's spec before
// provider defaulting, so the provider's annotation writes land in a map the
// caller shares. provider defaulting itself needs a reachable infra cluster
// and fails offline; the error must propagate without a partial write-back.
func TestDefaultAndValidateMachineSpecKubeVirt(t *testing.T) {
	wantLabels := map[string]string{"role": "worker"}

	tests := []struct {
		name            string
		annotations     map[string]string
		wantAnnotations map[string]string
	}{
		{
			name:            "nil annotations are initialized",
			wantAnnotations: map[string]string{},
		},
		{
			name:            "existing annotations are preserved",
			annotations:     map[string]string{"team": "dev-kkp"},
			wantAnnotations: map[string]string{"team": "dev-kkp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// keep the test hermetic; the provider falls back to this env var.
			t.Setenv("KUBEVIRT_KUBECONFIG", "")

			spec := machineSpecWithProviderConfig(t, providerconfig.CloudProviderKubeVirt, []byte("{}"))
			spec.Annotations = tt.annotations
			spec.Labels = map[string]string{"role": "worker"}

			ad := newTestAdmissionData(t)
			err := ad.defaultAndValidateMachineSpec(context.Background(), &spec)
			if err == nil || !strings.Contains(err.Error(), "failed to default machineSpec") {
				t.Fatalf("defaultAndValidateMachineSpec() error = %v, want the offline provider defaulting error", err)
			}

			if !reflect.DeepEqual(spec.Annotations, tt.wantAnnotations) {
				t.Errorf("annotations = %v, want %v", spec.Annotations, tt.wantAnnotations)
			}
			if !reflect.DeepEqual(spec.Labels, wantLabels) {
				t.Errorf("labels = %v, want %v untouched on the error path", spec.Labels, wantLabels)
			}
		})
	}
}

func TestValidatePublicKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		err  error
	}{
		{
			name: "valid keys",
			keys: []string{
				//RSA
				validRSA1024Key,
				validRSA2048Key,
				validRSA4096Key,

				// ECDSA
				validECDSA256Key,
				validECDSA384Key,
				validECDSA521Key,

				// DSA
				validDSA1024Key,
			},
		},
		{
			name: "invalid key",
			keys: []string{"some invalid key"},
			err:  errors.New(`invalid public key "some invalid key": ssh: no key found; last parsing error for ignored line: illegal base64 data at input byte 0`),
		},
		{
			name: "one of many is invalid",
			keys: []string{
				validRSA1024Key,
				"some invalid key",
			},
			err: errors.New(`invalid public key "some invalid key": ssh: no key found; last parsing error for ignored line: illegal base64 data at input byte 0`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePublicKeys(test.keys)
			if fmt.Sprint(err) != fmt.Sprint(test.err) {
				t.Errorf("Expected error to be\n%v\ninstead got\n%v", test.err, err)
			}
		})
	}
}
