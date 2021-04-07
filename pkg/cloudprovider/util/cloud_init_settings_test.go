/*
Copyright 2021 The Machine Controller Authors.

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

package util

import (
	"context"
	"io/ioutil"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testData = []struct {
	name              string
	userdata          string
	secret            *corev1.Secret
	expectedToken     string
	expectedCloudInit string
}{
	{
		name:     "bootstrap_cloud_init_generating",
		userdata: "./testdata/userdata.yaml",
		secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jwtTokenNamePrefix,
				Namespace: CloudInitNamespace,
			},
			Data: map[string][]byte{
				"token": []byte("eyTestToken"),
			},
		},
		expectedToken:     "eyTestToken",
		expectedCloudInit: "./testdata/expected_userdata.yaml",
	},
}

func TestCloudInitGeneration(t *testing.T) {
	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.secret)

			token, err := ExtractCloudInitSettingsToken(context.Background(), fakeClient)
			if err != nil {
				t.Fatalf("failed to extarct token: %v", err)
			}
			if token != test.expectedToken {
				t.Fatalf("unexpected cloud-init token: wants %s got %s", test.expectedToken, token)
			}

			userdata, err := ioutil.ReadFile(test.userdata)
			if err != nil {
				t.Fatalf("failed to read userdata testing file: %v", err)
			}

			cloudInit, err := GenerateCloudInitGetterScript(token, test.secret.Name, string(userdata))
			if err != nil {
				t.Fatalf("failed to generate bootstrap cloud-init: %v", err)
			}

			expectedCloudInit, err := ioutil.ReadFile(test.expectedCloudInit)
			if err != nil {
				t.Fatalf("failed to read expected cloud-init testing file: %v", err)
			}

			if cloudInit == string(expectedCloudInit) {
				t.Fatalf("unexpected cloud-init: wants %s got %s", test.expectedCloudInit, cloudInit)

			}
		})
	}
}
