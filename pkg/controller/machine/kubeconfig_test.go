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

package controller

import (
	"bytes"
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateSecretExpirationAndGetToken(t *testing.T) {
	tests := []struct {
		initialExpirationTime time.Time
		shouldRenew           bool
	}{
		{
			initialExpirationTime: time.Now().Add(1 * time.Hour),
			shouldRenew:           false,
		},
		{
			initialExpirationTime: time.Now().Add(25 * time.Minute),
			shouldRenew:           true,
		},
		{
			initialExpirationTime: time.Now().Add(-25 * time.Minute),
			shouldRenew:           true,
		},
	}

	reconciler := Reconciler{}

	for _, testCase := range tests {
		ctx := context.Background()
		secret := &corev1.Secret{}
		secret.Name = "secret"
		secret.Namespace = metav1.NamespaceSystem
		data := map[string][]byte{}
		data[tokenSecretKey] = []byte("tokenSecret")
		data[tokenIDKey] = []byte("tokenID")
		data[expirationKey] = []byte(testCase.initialExpirationTime.Format(time.RFC3339))
		secret.Data = data
		reconciler.client = ctrlruntimefake.
			NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(secret).
			Build()

		if _, err := reconciler.updateSecretExpirationAndGetToken(ctx, secret); err != nil {
			t.Fatalf("Unexpected error running updateSecretExpirationAndGetToken: %v", err)
		}

		updatedSecret := &corev1.Secret{}
		if err := reconciler.client.Get(ctx, types.NamespacedName{
			Namespace: metav1.NamespaceSystem,
			Name:      "secret",
		}, updatedSecret); err != nil {
			t.Fatalf("Unsexpected error getting secret: %v", err)
		}

		if testCase.shouldRenew &&
			bytes.Equal(updatedSecret.Data[expirationKey], []byte(testCase.initialExpirationTime.Format(time.RFC3339))) {
			t.Errorf("Error, token secret did not update but was expected to!")
		}

		if !testCase.shouldRenew &&
			!bytes.Equal(updatedSecret.Data[expirationKey], []byte(testCase.initialExpirationTime.Format(time.RFC3339))) {
			t.Errorf("Error, token secret was expected to get updated, but did not happen!")
		}

		expirationTimeParsed, err := time.Parse(time.RFC3339, string(secret.Data[expirationKey]))
		if err != nil {
			t.Fatalf("Failed to parse timestamp from secret: %v", err)
		}

		if time.Until(expirationTimeParsed).Minutes() < 0 {
			t.Errorf("Error, secret expiration is in the past!")
		}
	}
}
