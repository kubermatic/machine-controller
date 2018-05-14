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
	tokenFormatter                         = "%s.s"
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
		StringData: map[string]string{
			"description":                    "bootstrap token for " + name,
			tokenIDKey:                       tokenID,
			tokenSecretKey:                   tokenSecret,
			"expiration":                     metav1.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"usage-bootstrap-authentication": "true",
			"usage-bootstrap-signing":        "true",
			"auth-extra-groups":              "system:bootstrappers:machine-controller:default-node-token",
		},
	}

	_, err = c.kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Create(&secret)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(tokenFormatter, tokenID, tokenSecret), nil
}

func (c *Controller) updateSecretExpirationAndGetToken(secret *v1.Secret) (string, error) {
	if secret.StringData == nil {
		secret.StringData = map[string]string{}
	}
	secret.StringData["expiration"] = metav1.Now().Add(24 * time.Hour).Format(time.RFC3339)
	updatedSecret, err := c.kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Update(secret)
	if err != nil {
		return "", err

	}
	tokenID := updatedSecret.StringData[tokenIDKey]
	tokenSecret := updatedSecret.StringData[tokenSecretKey]
	return fmt.Sprintf(tokenFormatter, tokenID, tokenSecret), nil
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
