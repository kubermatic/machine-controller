package nodecsrapprover

import (
	"context"
	"sync"
	"testing"

	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/typed/certificates/v1beta1/fake"
	fakeclienttest "k8s.io/client-go/testing"

	k8sclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testCertificateRequest = `-----BEGIN CERTIFICATE REQUEST-----
MIIBnTCCAUQCAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEAvwSO5A7Thh4Dw1KejVf
JwvVeTXCbe5i/AzQr1DRO2g9Hsd/3iwFc27OX29PJ6tXt9wNVG9dn+D7fLuvRIdY
4KCBiDCBhQYJKoZIhvcNAQkOMXgwdjB0BgNVHREEbTBrgjBlYzItNTItNDctOTct
MTExLmV1LXdlc3QtMy5jb21wdXRlLmFtYXpvbmF3cy5jb22CK2lwLTE3Mi0zMS0x
MTQtNDguZXUtd2VzdC0zLmNvbXB1dGUuaW50ZXJuYWyHBKwfcjCHBDQvYW8wCgYI
KoZIzj0EAwIDRwAwRAIgLdF0Ud9UHmE3Ezxovw5oafCAKYiCE/EirpNXkUHee80C
IDjKG4ahwgDJQRtpmGqufjFBqrVVI3DxEFvt2RATJ3HA
-----END CERTIFICATE REQUEST-----`
)

func TestReconciler_Reconcile(t *testing.T) {
	reaction := &fakeApproveReaction{}
	simpleReactor := &fakeclienttest.SimpleReactor{
		Verb:     "*",
		Resource: "*",
		Reaction: reaction.approveReaction,
	}
	testCases := []struct {
		name          string
		reconciler    reconciler
	}{
		{
			name: "test approving a created certificate",
			reconciler: reconciler{
				Client: k8sclientfake.NewFakeClient(&certificatesv1beta1.CertificateSigningRequest{
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "123456",
						Name:            "csr",
						Namespace:       metav1.NamespaceSystem,
					},
					Spec: certificatesv1beta1.CertificateSigningRequestSpec{
						Request: []byte(testCertificateRequest),
						Usages: []certificatesv1beta1.KeyUsage{
							certificatesv1beta1.UsageDigitalSignature,
							certificatesv1beta1.UsageKeyEncipherment,
							certificatesv1beta1.UsageServerAuth,
						},
						Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
						Groups: []string{
							"system:nodes",
						},
					},
				}),
				certClient: &fake.FakeCertificateSigningRequests{
					Fake: &fake.FakeCertificatesV1beta1{
						Fake: &fakeclienttest.Fake{
							RWMutex:       sync.RWMutex{},
							ReactionChain: []fakeclienttest.Reactor{simpleReactor},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if err := tc.reconciler.reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "csr", Namespace: metav1.NamespaceSystem}}); err != nil {
				t.Fatalf("failed executing test: %v", err)
			}

			for _, cond := range reaction.expectedCSR.Status.Conditions {
				if cond.Type != certificatesv1beta1.CertificateApproved {
					t.Fatalf("failed updating csr condition")
				}
			}
		})
	}
}

type fakeApproveReaction struct {
	expectedCSR *certificatesv1beta1.CertificateSigningRequest
}

func (f *fakeApproveReaction) approveReaction(action fakeclienttest.Action) (bool, runtime.Object, error) {
	f.expectedCSR = action.(fakeclienttest.UpdateActionImpl).Object.(*certificatesv1beta1.CertificateSigningRequest)
	return true, f.expectedCSR, nil
}
