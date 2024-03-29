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

package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/pborman/uuid"
	"golang.org/x/crypto/ssh"
)

const privateRSAKeyBitSize = 4096

// Pubkey is only used to create temporary key pairs, thus we
// do not need the Private key
// The reason for not hardcoding a random public key is that
// it would look like a backdoor.
type Pubkey struct {
	Name           string
	PublicKey      string
	FingerprintMD5 string
}

func NewKey() (*Pubkey, error) {
	tmpRSAKeyPair, err := rsa.GenerateKey(rand.Reader, privateRSAKeyBitSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create private RSA key: %w", err)
	}

	if err := tmpRSAKeyPair.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate private RSA key: %w", err)
	}

	pubKey, err := ssh.NewPublicKey(&tmpRSAKeyPair.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ssh public key: %w", err)
	}

	return &Pubkey{
		Name:           uuid.New(),
		PublicKey:      string(ssh.MarshalAuthorizedKey(pubKey)),
		FingerprintMD5: ssh.FingerprintLegacyMD5(pubKey),
	}, nil
}
