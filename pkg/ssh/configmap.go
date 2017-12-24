package ssh

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	publicKeyDataIndex  = "public-key"
	privateKeyDataIndex = "private-key"
)

func EnsureSSHKeypairSecret(client kubernetes.Interface) (*KeyPair, error) {
	name := "machine-ssh-keypair"

	secret, err := client.CoreV1().Secrets(metav1.NamespaceSystem).Get(name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(4).Info("generating master ssh keypair")
			keypair, err := NewKeyPair()
			if err != nil {
				return nil, fmt.Errorf("failed to generate ssh keypair: %v", err)
			}

			secret := v1.Secret{}
			secret.Name = name
			secret.Type = v1.SecretTypeOpaque

			secret.Data = map[string][]byte{
				privateKeyDataIndex: keypair.PrivateKey,
				publicKeyDataIndex:  keypair.PublicKey,
			}

			_, err = client.CoreV1().Secrets(metav1.NamespaceSystem).Create(&secret)
			if err != nil {
				return nil, err
			}
			return keypair, nil
		}
		return nil, err
	}

	return keypairFromSecret(secret)
}

func keypairFromSecret(secret *v1.Secret) (*KeyPair, error) {
	b, _ := pem.Decode(secret.Data[privateKeyDataIndex])
	_, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	_, _, _, _, err = ssh.ParseAuthorizedKey(secret.Data[publicKeyDataIndex])
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	return &KeyPair{
		PublicKey:  secret.Data[publicKeyDataIndex],
		PrivateKey: secret.Data[privateKeyDataIndex],
	}, nil
}
