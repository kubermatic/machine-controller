package controller

import (
	"bytes"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestUpdateSecretExpirationAndGetToken(t *testing.T) {
	tests := []struct {
		initialExperirationTime time.Time
		shouldRenew             bool
	}{
		{
			initialExperirationTime: time.Now().Add(1 * time.Hour),
			shouldRenew:             false,
		},
		{
			initialExperirationTime: time.Now().Add(25 * time.Minute),
			shouldRenew:             true,
		},
		{
			initialExperirationTime: time.Now().Add(-25 * time.Minute),
			shouldRenew:             true,
		},
	}
	controller := Controller{}

	for _, testCase := range tests {
		secret := &corev1.Secret{}
		secret.Name = "secret"
		secret.Namespace = metav1.NamespaceSystem
		data := map[string][]byte{}
		data[tokenSecretKey] = []byte("tokenSecret")
		data[tokenIDKey] = []byte("tokenID")
		data[expirationKey] = []byte(testCase.initialExperirationTime.Format(time.RFC3339))
		secret.Data = data
		controller.kubeClient = kubefake.NewSimpleClientset(runtime.Object(secret))

		_, err := controller.updateSecretExpirationAndGetToken(secret)
		if err != nil {
			t.Fatalf("Unexpected error running updateSecretExpirationAndGetToken: %v", err)
		}

		updatedSecret, err := controller.kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Get("secret", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Unsexpected error getting secret: %v", err)
		}

		if testCase.shouldRenew &&
			bytes.Equal(updatedSecret.Data[expirationKey], []byte(testCase.initialExperirationTime.Format(time.RFC3339))) {
			t.Errorf("Error, token secret did not update but was expected to!")
		}

		if !testCase.shouldRenew &&
			!bytes.Equal(updatedSecret.Data[expirationKey], []byte(testCase.initialExperirationTime.Format(time.RFC3339))) {
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
