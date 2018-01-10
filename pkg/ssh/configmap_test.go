package ssh

import (
	"crypto/rsa"
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"k8s.io/client-go/kubernetes"
)

func generateSecretWithKey(b []byte) runtime.Object {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string][]byte{
			privateKeyDataIndex: b,
		},
	}
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
				client: fakeClientFactory(generateSecretWithKey(nil)),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fake basis client without a malformed key",
			args: args{
				client: fakeClientFactory(generateSecretWithKey(nil)),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EnsureSSHKeypairSecret(tt.args.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureSSHKeypairSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EnsureSSHKeypairSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
