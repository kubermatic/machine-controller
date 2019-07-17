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
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretTypeBootstrapToken corev1.SecretType = "bootstrap.kubernetes.io/token"
	machineNameLabelKey      string            = "machine.k8s.io/machine.name"
	tokenIDKey               string            = "token-id"
	tokenSecretKey           string            = "token-secret"
	expirationKey            string            = "expiration"
	tokenFormatter           string            = "%s.%s"
)

func (r *Reconciler) createBootstrapKubeconfig(name string) (*clientcmdapi.Config, error) {
	var token string
	var err error

	if r.bootstrapTokenServiceAccountName != nil {
		token, err = r.getTokenFromServiceAccount(*r.bootstrapTokenServiceAccountName)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from ServiceAccount %s/%s: %v", r.bootstrapTokenServiceAccountName.Namespace, r.bootstrapTokenServiceAccountName.Name, err)
		}
	} else {
		token, err = r.createBootstrapToken(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create bootstrap token: %v", err)
		}
	}

	infoKubeconfig, err := r.kubeconfigProvider.GetKubeconfig()
	if err != nil {
		return nil, err
	}

	outConfig := infoKubeconfig.DeepCopy()

	outConfig.AuthInfos = map[string]*clientcmdapi.AuthInfo{
		"": {
			Token: token,
		},
	}

	return outConfig, nil
}

func (r *Reconciler) getTokenFromServiceAccount(name types.NamespacedName) (string, error) {
	sa := &corev1.ServiceAccount{}
	if err := r.client.Get(r.ctx, name, sa); err != nil {
		return "", fmt.Errorf("failed to get serviceAccount %q: %v", name.String(), err)
	}
	for _, serviceAccountSecretName := range sa.Secrets {
		serviceAccountSecret := &corev1.Secret{}
		if err := r.client.Get(r.ctx, types.NamespacedName{Namespace: sa.Namespace, Name: serviceAccountSecretName.Name}, serviceAccountSecret); err != nil {
			return "", fmt.Errorf("failed to get serviceAccountSecret: %v", err)
		}
		if serviceAccountSecret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}
		return string(serviceAccountSecret.Data[corev1.ServiceAccountTokenKey]), nil
	}
	return "", errors.New("no serviceAccountSecret found")
}

func (r *Reconciler) createBootstrapToken(name string) (string, error) {
	existingSecret, err := r.getSecretIfExists(name)
	if err != nil {
		return "", err
	}
	if existingSecret != nil {
		return r.updateSecretExpirationAndGetToken(existingSecret)
	}

	tokenID := rand.String(6)
	tokenSecret := rand.String(16)

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("bootstrap-token-%s", tokenID),
			Namespace: metav1.NamespaceSystem,
			Labels:    map[string]string{machineNameLabelKey: name},
		},
		Type: secretTypeBootstrapToken,
		Data: map[string][]byte{
			"description":                    []byte("bootstrap token for " + name),
			tokenIDKey:                       []byte(tokenID),
			tokenSecretKey:                   []byte(tokenSecret),
			expirationKey:                    []byte(metav1.Now().Add(1 * time.Hour).Format(time.RFC3339)),
			"usage-bootstrap-authentication": []byte("true"),
			"usage-bootstrap-signing":        []byte("true"),
			"auth-extra-groups":              []byte("system:bootstrappers:machine-controller:default-node-token"),
		},
	}

	if err := r.client.Create(r.ctx, &secret); err != nil {
		return "", fmt.Errorf("failed to create bootstrap token secret: %v", err)
	}

	return fmt.Sprintf(tokenFormatter, tokenID, tokenSecret), nil
}

func (r *Reconciler) updateSecretExpirationAndGetToken(secret *corev1.Secret) (string, error) {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	tokenID := string(secret.Data[tokenIDKey])
	tokenSecret := string(secret.Data[tokenSecretKey])
	token := fmt.Sprintf(tokenFormatter, tokenID, tokenSecret)

	expirationTime, err := time.Parse(time.RFC3339, string(secret.Data[expirationKey]))
	if err != nil {
		return "", err
	}

	//If the token is close to expire, reset it's expiration time
	if time.Until(expirationTime).Minutes() < 30 {
		secret.Data[expirationKey] = []byte(metav1.Now().Add(1 * time.Hour).Format(time.RFC3339))
	} else {
		return token, nil
	}

	if err := r.client.Update(r.ctx, secret); err != nil {
		return "", fmt.Errorf("failed to update secret: %v", err)
	}
	return token, nil
}

func (r *Reconciler) getSecretIfExists(name string) (*corev1.Secret, error) {
	req, err := labels.NewRequirement(machineNameLabelKey, selection.Equals, []string{name})
	if err != nil {
		return nil, err
	}
	selector := labels.NewSelector().Add(*req)
	secrets := &corev1.SecretList{}
	if err := r.client.List(r.ctx, &ctrlruntimeclient.ListOptions{
		Namespace:     metav1.NamespaceSystem,
		LabelSelector: selector}, secrets); err != nil {
		return nil, err
	}

	if len(secrets.Items) == 0 {
		return nil, nil
	}
	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("expected to find exactly one secret for the given machine name =%s but found %d", name, len(secrets.Items))
	}
	return &secrets.Items[0], nil
}
