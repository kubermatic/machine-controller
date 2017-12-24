package ssh

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

func NewKeyPair() (keyPair *KeyPair, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %v", err)
	}

	if err := priv.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate private key: %v", err)
	}

	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	privBuf := bytes.Buffer{}
	err = pem.Encode(&privBuf, privateKeyPEM)
	if err != nil {
		return nil, err
	}

	pubSSH, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create publickey from private key: %v", err)
	}

	return &KeyPair{
		PrivateKey: privBuf.Bytes(),
		PublicKey:  ssh.MarshalAuthorizedKey(pubSSH),
	}, nil
}
