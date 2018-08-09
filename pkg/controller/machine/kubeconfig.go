package controller

import (
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/rand"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	secretTypeBootstrapToken v1.SecretType = "bootstrap.kubernetes.io/token"
	machineNameLabelKey                    = "machine.k8s.io/machine.name"
	tokenIDKey                             = "token-id"
	tokenSecretKey                         = "token-secret"
	expirationKey                          = "expiration"
	tokenFormatter                         = "%s.%s"
)

func (c *Controller) createBootstrapKubeconfig(name string) (*clientcmdapi.Config, error) {
	token, err := c.createBootstrapToken(name)
	if err != nil {
		return nil, err
	}

	infoKubeconfig, err := c.kubeconfigProvider.GetKubeconfig()
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

func (c *Controller) createBootstrapToken(name string) (string, error) {
	existingSecret, err := c.getSecretIfExists(name)
	if err != nil {
		return "", err
	}
	if existingSecret != nil {
		return c.updateSecretExpirationAndGetToken(existingSecret)
	}

	tokenID := rand.String(6)
	tokenSecret := rand.String(16)

	secret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("bootstrap-token-%s", tokenID),
			Labels: map[string]string{machineNameLabelKey: name},
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

	_, err = c.kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Create(&secret)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(tokenFormatter, tokenID, tokenSecret), nil
}

func (c *Controller) updateSecretExpirationAndGetToken(secret *v1.Secret) (string, error) {
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

	_, err = c.kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Update(secret)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (c *Controller) getSecretIfExists(name string) (*v1.Secret, error) {
	req, err := labels.NewRequirement(machineNameLabelKey, selection.Equals, []string{name})
	if err != nil {
		return nil, err
	}

	selector := labels.NewSelector().Add(*req)
	secrets, err := c.secretSystemNsLister.List(selector)
	if err != nil {
		return nil, err
	}

	if len(secrets) == 0 {
		return nil, nil
	}
	if len(secrets) > 1 {
		return nil, fmt.Errorf("expected to find exactly one secret for the given machine name =%s but found %d", name, len(secrets))
	}
	return secrets[0], nil
}
