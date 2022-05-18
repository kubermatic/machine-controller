/*
Copyright 2022 The Machine Controller Authors.

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

package gce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/runtime"
	fake2 "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testProviderSpec() map[string]interface{} {
	return map[string]interface{}{
		"caPublicKey":   "",
		"cloudProvider": "gce",
		"cloudProviderSpec": map[string]interface{}{
			"assignPublicIPAddress": true,
			"customImage":           "",
			"diskSize":              25,
			"diskType":              "pd-standard",
			"machineType":           "e2-highcpu-2",
			"multizone":             false,
			"network":               "global/networks/default",
			"preemptible":           false,
			"provisioningModel":     "STANDARD",
			"regional":              false,
			"serviceAccount":        "",
			"subnetwork":            "",
			"tags": []string{
				"kubernetes-cluster-kdlj8sn58d",
				"system-cluster-kdlj8sn58d",
				"system-project-sszxpzjcnm",
			},
			"zone": "europe-west2-a",
		},
		"operatingSystem": "ubuntu",
		"operatingSystemSpec": map[string]interface{}{
			"distUpgradeOnBoot": false,
		},
		"sshPublicKeys": []string{},
		"network":       map[string]interface{}{},
	}
}

func testServiceAccount() string {
	return base64.StdEncoding.EncodeToString([]byte(`{
  "type": "service_account",
  "project_id": "test-dev",
  "private_key_id": "testprivatekeyid",
  "private_key": "-----BEGIN PRIVATE KEY-----\n\nMIICXQIBAAKBgQCU6DqY/hqXOOjomOuf3ESiMBxxRCpnn+VTAOPeEOsKaNdAv3zB\n\nmy6KEgOlraAi45cR8Ow8R0UFBNMQaU6Bck99t34BGZSQxjMTFw11W9p0GROKZgqG\n\nobj1WiomRQuwy0D6Q90wRSRhvnawKHqIEDoGnQT+SceV5vb6yoLmZSBoFQIDAQAB\n\nAoGASQoIBBdPz7E4fS7VFJqkh7F1ohE/g4iooagkHT7LK1X1j2rdtNF7aHohk9iw\n\nXayo40H7fi2vKyEMrlYZDeGWH1/XHLIyTGUNo91J3HDRbfs0eJhHKxFsdD/a64yV\n\ndYJviM2nsBQkbCC08O3yVCc/0spB7xKSBlpgFaWTnwDj8AECQQDnjen0Or9C7c9N\n\nOQdkefGoRzD0ltwbJoOHmRz3s49TieRmQpX+XkcbR91BPkIzgbQs8tFatK5YQNDp\n\nDTdp/VoVAkEApKCoEv6hNdj3sjY1qGT2e2sNCKbgsJeXfPrMbotypmv2VgK4w0IE\n\nPA+Tysd6G3EojFooDlzAkG2hXsgie2BWAQJBAN/finnSLsdD63CrGaWgbO+Y3REt\n\npmMtqm94rtQiLAnFwSjJagHEHxWWNqn0ysbHuW7X2WfMVuAG0rTwTUpRZD0CQQCQ\n\nhY0nJ6vkdrV0GIzgaMnNLPxDNSSZQms1x4JCJV8f5DVb6oXCvCi1hUNMR/PVNXDQ\n\nTbFOcnSGFggNCgrjXn4BAkAgoDpFUVa5wLvkWQpTnKXv//xMG4fS3xmlDHi3xE8d\n\nMfEPCgKd8giHPaW0p4XtTAmhk1sdpuR2op4ZfDorCmEC\n\n-----END PRIVATE KEY-----\n",
  "client_email": "someguy@some.com",
  "client_id": "whateverthisis",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/sometest"
}
`))
}

type testMap map[string]interface{}

// with patches value of m at keypath with val e.g. keypath=x.y val=z then m[x][y] = z.
func (m testMap) with(keypath, val string) testMap {
	parts := strings.Split(keypath, ".")
	var curr interface{} = m
	for _, p := range parts[:len(parts)-1] {
		switch m[p].(type) {
		case map[string]interface{}, testMap:
			curr = m[p]
		}
	}

	switch x := curr.(type) {
	case map[string]interface{}: //, testMap:
		x[parts[len(parts)-1]] = val
	case testMap:
		x[parts[len(parts)-1]] = val
	}
	return m
}

func TestValidate(t *testing.T) {
	t.Setenv(envGoogleServiceAccount, testServiceAccount())
	defer os.Unsetenv(envGoogleServiceAccount)

	rawBytes := func(m map[string]interface{}) []byte {
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	p := New(providerconfig.NewConfigVarResolver(context.Background(), fake2.NewClientBuilder().Build()))
	tests := []struct {
		name      string
		mspec     v1alpha1.MachineSpec
		expectErr bool
	}{
		{
			"without IP family",
			v1alpha1.MachineSpec{
				ProviderSpec: v1alpha1.ProviderSpec{
					Value: &runtime.RawExtension{
						Raw: rawBytes(testProviderSpec()),
					},
				},
			},
			false,
		},
		{
			"empty IP family",
			v1alpha1.MachineSpec{
				ProviderSpec: v1alpha1.ProviderSpec{
					Value: &runtime.RawExtension{
						Raw: rawBytes(testMap(testProviderSpec()).
							with("network.ipFamily", ""),
						),
					},
				},
			},
			false,
		},
		{
			"with IP family",
			v1alpha1.MachineSpec{
				ProviderSpec: v1alpha1.ProviderSpec{
					Value: &runtime.RawExtension{
						Raw: rawBytes(testMap(testProviderSpec()).
							with("network.ipFamily", "IPv4+IPv6"),
						),
					},
				},
			},
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := p.Validate(context.Background(), test.mspec)
			if (err != nil) != test.expectErr {
				t.Fatalf("expectedErr: %t, got: %v", test.expectErr, err)
			}
		})
	}
}
