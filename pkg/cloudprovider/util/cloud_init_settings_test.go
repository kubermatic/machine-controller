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
	"k8s.io/client-go/kubernetes/scheme"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testData = []struct {
	name              string
	userdata          string
	secret            *corev1.Secret
	expectedToken     string
	expectedAPIServer string
}{
	{
		name:     "bootstrap_cloud_init_generating",
		userdata: "./testdata/userdata.yaml",
		secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cloudInitGetterSecret,
				Namespace: CloudInitNamespace,
			},
			Data: map[string][]byte{
				"token": []byte("eyTestToken"),
			},
		},
		expectedToken:     "eyTestToken",
		expectedAPIServer: "https://88.99.224.97:6443",
	},
}

func TestCloudInitGeneration(t *testing.T) {
	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fakectrlruntimeclient.
				NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(test.secret).
				Build()

			userdata, err := ioutil.ReadFile(test.userdata)
			if err != nil {
				t.Fatalf("failed to read userdata testing file: %v", err)
			}

			token, apiServer, err := ExtractTokenAndAPIServer(context.Background(), string(userdata), fakeClient)
			if err != nil {
				t.Fatalf("failed to extarct token: %v", err)
			}
			if token != test.expectedToken {
				t.Fatalf("unexpected cloud-init token: wants %s got %s", test.expectedToken, token)
			}
			if apiServer != test.expectedAPIServer {
				t.Fatalf("unexpected cloud-init api-server: wants %s got %s", test.expectedAPIServer, apiServer)
			}
		})
	}
}
