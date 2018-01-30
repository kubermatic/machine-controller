package ssh

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func generateByteSlice(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func generateValidPEM() []byte {
	b := &bytes.Buffer{}
	pk, _ := NewPrivateKey("test")

	_ = pem.Encode(b, &pem.Block{Type: rsaPrivateKey, Bytes: x509.MarshalPKCS1PrivateKey(pk.key)})
	return b.Bytes()
}

func generateSecretWithCustomNameData(secretName string, data map[string][]byte) runtime.Object {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: metav1.NamespaceSystem,
		},
		Data: data,
	}
}

func generateSecretWithCustomName(secretName, keyName string, b []byte) runtime.Object {
	return generateSecretWithCustomNameData(secretName, map[string][]byte{privateKeyDataIndex: b, privateKeyNameIndex: []byte(keyName)})
}

func generateSecretWithKey(keyName string, b []byte) runtime.Object {
	return generateSecretWithCustomName(secretName, keyName, b)
}

func fakeClientFactory(objs ...runtime.Object) kubernetes.Interface {
	return fake.NewSimpleClientset(objs...)
}

func TestEnsureSSHKeypairSecret(t *testing.T) {
	type args struct {
		client kubernetes.Interface
	}
	tests := []struct {
		name    string
		args    args
		want    *rsa.PrivateKey
		wantErr bool
	}{
		{
			name: "nil client",
			args: args{
				client: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fake basis client without a key",
			args: args{
				client: fakeClientFactory(generateSecretWithKey("does-not-exist", nil)),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fake basis client with a malformed key",
			args: args{
				client: fakeClientFactory(generateSecretWithKey("malformed-key", generateByteSlice(2048))),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fake basis client with a valid key",
			args: args{
				client: fakeClientFactory(generateSecretWithKey("some-valid-key", generateValidPEM())),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "fake basis client with a valid key, but the wrong secrete name",
			args: args{
				client: fakeClientFactory(generateSecretWithCustomName("foo", "bar", generateValidPEM())),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "fake basis client with missing keydata in secret",
			args: args{
				client: fakeClientFactory(generateSecretWithCustomNameData(secretName, map[string][]byte{privateKeyNameIndex: []byte("my-key"), privateKeyDataIndex: nil})),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fake basis client with missing name in secret",
			args: args{
				client: fakeClientFactory(generateSecretWithCustomNameData(secretName, map[string][]byte{privateKeyNameIndex: []byte(""), privateKeyDataIndex: generateValidPEM()})),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EnsureSSHKeypairSecret("test", tt.args.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureSSHKeypairSecret() error = %+v, wantErr %+v", err, tt.wantErr)
				return
			}
		})
	}
}
