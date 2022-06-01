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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/scheme"
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
	// Keep this short, userdata is limited.
	contextIdentifier string = "c"
)

func (r *Reconciler) createBootstrapKubeconfig(ctx context.Context, name string) (*clientcmdapi.Config, error) {
	var token string
	var err error

	if r.bootstrapTokenServiceAccountName != nil {
		token, err = r.getTokenFromServiceAccount(ctx, *r.bootstrapTokenServiceAccountName)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from ServiceAccount %s/%s: %w", r.bootstrapTokenServiceAccountName.Namespace, r.bootstrapTokenServiceAccountName.Name, err)
		}
	} else {
		token, err = r.createBootstrapToken(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("failed to create bootstrap token: %w", err)
		}
	}

	infoKubeconfig, err := r.kubeconfigProvider.GetKubeconfig(ctx)
	if err != nil {
		return nil, err
	}

	outConfig := infoKubeconfig.DeepCopy()

	// Some consumers expect a valid `Contexts` map and the serialization
	// for the Context ignores empty string fields, hence we must make sure
	// both the Cluster and the User have a non-empty key.
	clusterContextName := ""
	// This is supposed to have a length of 1. We have code further down the
	// line that extracts the CA cert and errors out if that is not the case,
	// so we can simply iterate over it here.
	for key := range infoKubeconfig.Clusters {
		clusterContextName = key
	}
	cluster := outConfig.Clusters[clusterContextName].DeepCopy()
	delete(outConfig.Clusters, clusterContextName)
	outConfig.Clusters[contextIdentifier] = cluster

	outConfig.AuthInfos = map[string]*clientcmdapi.AuthInfo{
		contextIdentifier: {
			Token: token,
		},
	}

	// This is supposed to have a length of 1. We have code further down the
	// line that extracts the CA cert and errors out if that is not the case.
	//
	// This handles a very special case in which we want to override the API server
	// address that will be used in the `bootstrap-kubelet.conf` in the worker nodes for
	// our E2E tests that run in KIND clusters.
	if r.overrideBootstrapKubeletAPIServer != "" {
		for key := range outConfig.Clusters {
			outConfig.Clusters[key].Server = r.overrideBootstrapKubeletAPIServer
		}
	}

	outConfig.Contexts = map[string]*clientcmdapi.Context{contextIdentifier: {Cluster: contextIdentifier, AuthInfo: contextIdentifier}}
	outConfig.CurrentContext = contextIdentifier

	return outConfig, nil
}

func (r *Reconciler) getTokenFromServiceAccount(ctx context.Context, name types.NamespacedName) (string, error) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace}}
	raw, err := r.getAsUnstructured(ctx, sa)
	if err != nil {
		return "", fmt.Errorf("failed to get serviceAccount %q: %w", name.String(), err)
	}
	sa = raw.(*corev1.ServiceAccount)
	for _, serviceAccountSecretName := range sa.Secrets {
		serviceAccountSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: sa.Namespace, Name: serviceAccountSecretName.Name}}
		raw, err = r.getAsUnstructured(ctx, serviceAccountSecret)
		if err != nil {
			return "", fmt.Errorf("failed to get serviceAccountSecret: %w", err)
		}
		serviceAccountSecret = raw.(*corev1.Secret)
		if serviceAccountSecret.Type != corev1.SecretTypeServiceAccountToken {
			continue
		}
		return string(serviceAccountSecret.Data[corev1.ServiceAccountTokenKey]), nil
	}
	return "", errors.New("no serviceAccountSecret found")
}

func (r *Reconciler) createBootstrapToken(ctx context.Context, name string) (string, error) {
	existingSecret, err := r.getSecretIfExists(ctx, name)
	if err != nil {
		return "", err
	}
	if existingSecret != nil {
		return r.updateSecretExpirationAndGetToken(ctx, existingSecret)
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

	if err := r.client.Create(ctx, &secret); err != nil {
		return "", fmt.Errorf("failed to create bootstrap token secret: %w", err)
	}

	return fmt.Sprintf(tokenFormatter, tokenID, tokenSecret), nil
}

func (r *Reconciler) updateSecretExpirationAndGetToken(ctx context.Context, secret *corev1.Secret) (string, error) {
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

	// If the token is close to expire, reset it's expiration time
	if time.Until(expirationTime).Minutes() < 30 {
		secret.Data[expirationKey] = []byte(metav1.Now().Add(1 * time.Hour).Format(time.RFC3339))
	} else {
		return token, nil
	}

	if err := r.client.Update(ctx, secret); err != nil {
		return "", fmt.Errorf("failed to update secret: %w", err)
	}
	return token, nil
}

func (r *Reconciler) getSecretIfExists(ctx context.Context, name string) (*corev1.Secret, error) {
	req, err := labels.NewRequirement(machineNameLabelKey, selection.Equals, []string{name})
	if err != nil {
		return nil, err
	}
	selector := labels.NewSelector().Add(*req)
	secrets := &corev1.SecretList{}
	if err := r.client.List(ctx, secrets,
		&ctrlruntimeclient.ListOptions{
			Namespace:     metav1.NamespaceSystem,
			LabelSelector: selector}); err != nil {
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

// getAsUnstructured is a helper to get an object as unstrucuted.Unstructered from the client.
// The purpose of this is to avoid establishing a lister, which the cache-backed client automatically
// does. The object passed in must have name and namespace set. The returned object will
// be the same as the passed in one, if there was no error.
func (r *Reconciler) getAsUnstructured(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return nil, errors.New("can not assert object as metav1.Object")
	}
	kinds, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get kinds for object: %w", err)
	}
	if len(kinds) == 0 {
		return nil, fmt.Errorf("found no kind for object %t", obj)
	}
	apiVersion, kind := kinds[0].ToAPIVersionAndKind()

	target := &unstructured.Unstructured{}
	target.SetAPIVersion(apiVersion)
	target.SetKind(kind)
	name := types.NamespacedName{Name: metaObj.GetName(), Namespace: metaObj.GetNamespace()}

	if err := r.client.Get(ctx, name, target); err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	rawJSON, err := target.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unstructured.Unstructured: %w", err)
	}
	if err := json.Unmarshal(rawJSON, obj); err != nil {
		return nil, fmt.Errorf("failed to marshal unstructured.Unstructued into %T: %w", obj, err)
	}
	return obj, nil
}
