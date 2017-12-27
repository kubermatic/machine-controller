package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
)

func NewPrivateKey() (key *rsa.PrivateKey, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %v", err)
	}

	if err := priv.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate private key: %v", err)
	}

	return priv, nil
}
