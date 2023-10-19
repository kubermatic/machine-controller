/*
Copyright 2020 The Machine Controller Authors.

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

package nodecsrapprover

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
	Certificate generation (cfssl required):
cat <<EOF | cfssl genkey - | cfssljson -bare server
{
  "hosts": [
    "ip-172-31-114-48.eu-west-3.compute.internal",
    "192.0.2.24"
  ],
  "CN": "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
  "key": {
    "algo": "ecdsa",
    "size": 256
  },
  "names": [{
	"O": "system:nodes"
  }]
}
EOF
*/

const (
	testValidCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBYzCCAQoCAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEvt0lQhUzm7HRM+nK64Uj
B8Nm4N59ksjOUiEnH1nDamPTZXjVzhfUShKiaYTpGU11xZz2rZu97AGhZeiSfPI4
eqBPME0GCSqGSIb3DQEJDjFAMD4wPAYDVR0RBDUwM4IraXAtMTcyLTMxLTExNC00
OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRlcm5hbIcEwAACGDAKBggqhkjOPQQDAgNH
ADBEAiAenX66LOL8U0C6Oo5WAQLxErmImuKpN1lRj/qeYE3sYQIgGu1UpkyNmMqM
ZNeG8Y4JZvQejnbdkbToeB0yodC1tEE=
-----END CERTIFICATE REQUEST-----
`

	testInvalidCommonNameCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBMDCB1wIBADAmMRUwEwYDVQQKEwxzeXN0ZW06bm9kZXMxDTALBgNVBAMTBHRl
c3QwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAT4FVNaMAPVcv05WF+EQ3FbTmax
A3dIxE938g04uFvKFV00TggRaUW/obOo8BET0ADHmxA3QWM9XfX2UNI88VRLoE8w
TQYJKoZIhvcNAQkOMUAwPjA8BgNVHREENTAzgitpcC0xNzItMzEtMTE0LTQ4LmV1
LXdlc3QtMy5jb21wdXRlLmludGVybmFshwTAAAIYMAoGCCqGSM49BAMCA0gAMEUC
IQDqnqQLBH2nBVoz4P35lrSPVqAZYmg5HN145jMy3/GUFQIgIHJJ0U6sd8xbksMV
xW5irA6nWST0ILKeIJz7BBds2K0=
-----END CERTIFICATE REQUEST-----
`

	testMultipleOrganizationsCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBcDCCARcCAQAwZjEiMBMGA1UEChMMc3lzdGVtOm5vZGVzMAsGA1UEChMEdGVz
dDFAMD4GA1UEAxM3c3lzdGVtOm5vZGU6aXAtMTcyLTMxLTExNC00OC5ldS13ZXN0
LTMuY29tcHV0ZS5pbnRlcm5hbDBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABNF9
okm72Aez+e9qeylkTGYQOTfEgSRRgmgyOtkqug/QwxkR0adlyIJcKiX/DZ4Eiqhe
tFgq4c4vBNKKg6SKxWKgTzBNBgkqhkiG9w0BCQ4xQDA+MDwGA1UdEQQ1MDOCK2lw
LTE3Mi0zMS0xMTQtNDguZXUtd2VzdC0zLmNvbXB1dGUuaW50ZXJuYWyHBMAAAhgw
CgYIKoZIzj0EAwIDRwAwRAIgTKGXyZ8WEoPWba8varaH1RTKjLVq8Xa0iDMQGCRx
wDQCIBkAk4Z5KiOQMWEH8gH+cl5EVprL4bCEyPnnNF8gofpz
-----END CERTIFICATE REQUEST-----
`

	testNoOrganizationCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBTTCB8wIBADBCMUAwPgYDVQQDEzdzeXN0ZW06bm9kZTppcC0xNzItMzEtMTE0
LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmludGVybmFsMFkwEwYHKoZIzj0CAQYIKoZI
zj0DAQcDQgAEMA3EgzRCWIK1NbZ3WE4w3SRX4AXub49aGs1f7emlmGjs7smhN/NB
fzbPH/eMGkVB8A10+DyuHw51XCLufUrbJKBPME0GCSqGSIb3DQEJDjFAMD4wPAYD
VR0RBDUwM4IraXAtMTcyLTMxLTExNC00OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRl
cm5hbIcEwAACGDAKBggqhkjOPQQDAgNJADBGAiEAtuVHYeKM8uNtJy0kFCFJmsIy
g/X2uvkjAnSLjizhF3oCIQCfwZ5jQy5JUufOYORk0oEl7DAln10PkRFICBoDcVtt
wQ==
-----END CERTIFICATE REQUEST-----
`

	testInvalidOrganizationCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBWzCCAQICAQAwUTENMAsGA1UEChMEdGVzdDFAMD4GA1UEAxM3c3lzdGVtOm5v
ZGU6aXAtMTcyLTMxLTExNC00OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRlcm5hbDBZ
MBMGByqGSM49AgEGCCqGSM49AwEHA0IABP9SWPd4Tf8zWuzTTBratQRFwC6dXyyq
IltYaTny5+pZxwiSzZCuLSYe1mSPQvBLjfMcMJiDIeH82oHXPehxPR+gTzBNBgkq
hkiG9w0BCQ4xQDA+MDwGA1UdEQQ1MDOCK2lwLTE3Mi0zMS0xMTQtNDguZXUtd2Vz
dC0zLmNvbXB1dGUuaW50ZXJuYWyHBMAAAhgwCgYIKoZIzj0EAwIDRwAwRAIgNui6
Ch0hJUx9r3a1VTOMRa5BDJ6z02hizuSC4rMg3N4CIEmKExaOFYT5lwOs5ZYaQ4xc
6usii8YdNQ7Y0WlreYTQ
-----END CERTIFICATE REQUEST-----
`

	testInvalidDNSNameCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBYzCCAQoCAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEI0vty8uehZOaXxjluaEi
DaTBhB0rplD92+Mi8Xt36nCq+BmrtXBIst1Cp8jzx/AESEuC/06T1FoZnXir5H5k
KKBPME0GCSqGSIb3DQEJDjFAMD4wPAYDVR0RBDUwM4IraXAtMTcyLTMxLTExNC00
OC5ldS1lYXN0LTMuY29tcHV0ZS5pbnRlcm5hbIcEwAACGDAKBggqhkjOPQQDAgNH
ADBEAiARKzqSf5A4K9mSpfN5BZkvagN2hpSuCrNh969aMHKWGgIgLMlqn6GED5cS
rHy6Ox5XuLIEfuyO2lw4Tpse1T0MN5c=
-----END CERTIFICATE REQUEST-----
`

	testInvalidIPAddressCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBYzCCAQoCAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5bGjubaUuMpo5IHK/nfm
fPoG/uPDkDUtx9wugPaNGd8vn5Mu/3QMf3NuD5kXRY4RCWl9w4vddkLdT+hSekP1
i6BPME0GCSqGSIb3DQEJDjFAMD4wPAYDVR0RBDUwM4IraXAtMTcyLTMxLTExNC00
OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRlcm5hbIcEwAACAjAKBggqhkjOPQQDAgNH
ADBEAiB9myW6lAcQKX2WCsfriwK3aIMzgY4jCkr8tPV1oZeLLQIgbwlqgQcAOQqC
/6Fr0hK/AIDlykBtDKkNYDgm3n3Oqqc=
-----END CERTIFICATE REQUEST-----
`

	testAdditionalDNSNameCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBkTCCATcCAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEA9/qlNDkHRRv/MMvqGDv
glwVIepZ1nXpA72XmwT9eKGKIGhvYkU5y6HkpkLv45niTxhhYN4EbSwAC7651ZsK
6qB8MHoGCSqGSIb3DQEJDjFtMGswaQYDVR0RBGIwYIIraXAtMTcyLTMxLTExNC00
OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRlcm5hbIIraXAtMTcyLTMxLTExNC00OC5l
dS1lYXN0LTMuY29tcHV0ZS5pbnRlcm5hbIcEwAACGDAKBggqhkjOPQQDAgNIADBF
AiAr5gSITnVYXOFmvEU+mcHk8w/8/cbKwMHWtLZ0ZLN+igIhAP0BE//qz0lAFINS
Su19WQIpe9VR/dGJvXfsgaLjyPyJ
-----END CERTIFICATE REQUEST-----
`

	testAdditionalIPAddressCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBazCCARACAQAwWTEVMBMGA1UEChMMc3lzdGVtOm5vZGVzMUAwPgYDVQQDEzdz
eXN0ZW06bm9kZTppcC0xNzItMzEtMTE0LTQ4LmV1LXdlc3QtMy5jb21wdXRlLmlu
dGVybmFsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEPg/O69m6AjfXB8XoKjAx
2RqAdKzt1aJ9u0rbOcoa2QkTcFeMY2GbAZ/6IVWnTDsqORJeJrHP5WYNUFgBjQ2c
gaBVMFMGCSqGSIb3DQEJDjFGMEQwQgYDVR0RBDswOYIraXAtMTcyLTMxLTExNC00
OC5ldS13ZXN0LTMuY29tcHV0ZS5pbnRlcm5hbIcEwAACGIcEwAACGTAKBggqhkjO
PQQDAgNJADBGAiEA3HDpVRYYgcmdCzq5o6mwkMzegZ0P0aZNPdCQLyJt3GoCIQDo
/iL1+piFJXAOI2GjsZNpeQJ4rPJ7l/t95tzgcVAUtw==
-----END CERTIFICATE REQUEST-----
`

	testNoDNSNameCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIIBNjCB3QIBADBZMRUwEwYDVQQKEwxzeXN0ZW06bm9kZXMxQDA+BgNVBAMTN3N5
c3RlbTpub2RlOmlwLTE3Mi0zMS0xMTQtNDguZXUtd2VzdC0zLmNvbXB1dGUuaW50
ZXJuYWwwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAATjbIZVQA/A+lki63bMCgVR
UrkCqtVufWtKU5KPTD9BGZ6/dNW/uUdVftLe7Vp4+8cp2YoZIQA5NfMGYMtcE9PV
oCIwIAYJKoZIhvcNAQkOMRMwETAPBgNVHREECDAGhwTAAAIYMAoGCCqGSM49BAMC
A0gAMEUCIQDxZpheeDW9azYm0T0OoOyFCFslJsRf9uvJM66byWr0KwIgduhJVRog
ELloyxg3KHxPJi8TYkTPkB+Pc9XiUGmhkzE=
-----END CERTIFICATE REQUEST-----
`
)

func TestValidateCSRObject(t *testing.T) {
	testCases := []struct {
		name     string
		csr      *certificatesv1.CertificateSigningRequest
		nodeName string
		err      error
	}{
		{
			name: "test validating a valid csr",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testValidCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			nodeName: "ip-172-31-114-48.eu-west-3.compute.internal",
			err:      nil,
		},
		{
			name: "test validating a valid csr with more than 2 groups",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testValidCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
						"test-group",
					},
				},
			},
			nodeName: "ip-172-31-114-48.eu-west-3.compute.internal",
			err:      nil,
		},
		{
			name: "test validating an invalid csr (missing prefix)",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username: "node:ip-172-31-114-48.eu-west-3.compute.internal",
				},
			},
			nodeName: "",
			err:      fmt.Errorf("username must have the '%s' prefix", nodeUserPrefix),
		},
		{
			name: "validate with missing node name in username",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username: "system:node:",
				},
			},
			nodeName: "",
			err:      fmt.Errorf("node name is empty"),
		},
		{
			name: "validate csr with with missing system:nodes group",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:authenticated",
					},
				},
			},
			nodeName: "",
			err:      fmt.Errorf("there are less than 2 groups"),
		},
		{
			name: "validate csr with two invalid groups",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"group-1",
						"group-2",
					},
				},
			},
			nodeName: "",
			err:      fmt.Errorf("'%s' and/or '%s' are not in its groups", nodeGroup, authenticatedGroup),
		},
		{
			name: "validate csr with usages not matching expected usages",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testValidCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			nodeName: "",
			err:      fmt.Errorf("usage %v is not in the list of allowed usages (%v)", "client auth", allowedUsages),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &reconciler{}
			nodeName, err := r.validateCSRObject(tc.csr)
			if err != nil && err.Error() != tc.err.Error() {
				t.Errorf("expected error '%v', but got '%v'", tc.err, err)
			} else if err == nil && tc.err != nil {
				t.Errorf("expected error '%v'", tc.err)
			}
			if nodeName != tc.nodeName {
				t.Errorf("expected node name '%s', but got '%s'", tc.nodeName, nodeName)
			}
		})
	}
}

func TestValidateX509CSR(t *testing.T) {
	machine := v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: v1alpha1.MachineSpec{},
		Status: v1alpha1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       "ip-172-31-114-48.eu-west-3.compute.internal",
			},
			Addresses: []corev1.NodeAddress{
				{
					Address: "ip-172-31-114-48.eu-west-3.compute.internal",
					Type:    corev1.NodeExternalDNS,
				},
				{
					Address: "192.0.2.24",
					Type:    corev1.NodeExternalIP,
				},
			},
		},
	}

	testCases := []struct {
		name    string
		csr     *certificatesv1.CertificateSigningRequest
		machine v1alpha1.Machine
		err     error
	}{
		{
			name: "validate valid csr",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testValidCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     nil,
		},
		{
			name: "validate csr with no dns name in machine object",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testNoDNSNameCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machine",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: v1alpha1.MachineSpec{},
				Status: v1alpha1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						APIVersion: "v1",
						Kind:       "Node",
						Name:       "ip-172-31-114-48.eu-west-3.compute.internal",
					},
					Addresses: []corev1.NodeAddress{
						{
							Address: "192.0.2.24",
							Type:    corev1.NodeExternalIP,
						},
					},
				},
			},
			err: nil,
		},
		{
			name: "validate csr with different common name than username",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testInvalidCommonNameCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("commonName '%s' is different then CSR username '%s'", "test", "system:node:ip-172-31-114-48.eu-west-3.compute.internal"),
		},
		{
			name: "validate csr with different username than common name",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testValidCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "test-username",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("commonName '%s' is different then CSR username '%s'", "system:node:ip-172-31-114-48.eu-west-3.compute.internal", "test-username"),
		},
		{
			name: "validate csr with multiple organizations",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testMultipleOrganizationsCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("expected only one organization but got %d instead", 2),
		},
		{
			name: "validate csr with no organization",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testNoOrganizationCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("expected only one organization but got %d instead", 0),
		},
		{
			name: "validate csr with organization not matching system:nodes",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testInvalidOrganizationCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("organization '%s' doesn't match node group '%s'", "test", nodeGroup),
		},
		{
			name: "validate csr with invalid dns name",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testInvalidDNSNameCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("dns name '%s' cannot be associated with node '%s'", "ip-172-31-114-48.eu-east-3.compute.internal", machine.Status.NodeRef.Name),
		},
		{
			name: "validate csr with invalid ip address",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testInvalidIPAddressCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("ip address '%v' cannot be associated with node '%s'", "192.0.2.2", machine.Status.NodeRef.Name),
		},
		{
			name: "validate csr with dns name not defined in the machine object",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testAdditionalDNSNameCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("dns name '%s' cannot be associated with node '%s'", "ip-172-31-114-48.eu-east-3.compute.internal", machine.Status.NodeRef.Name),
		},
		{
			name: "validate csr with ip address not defined in the machine object",
			csr: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csr",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: []byte(testAdditionalIPAddressCSR),
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Username: "system:node:ip-172-31-114-48.eu-west-3.compute.internal",
					Groups: []string{
						"system:nodes",
						"system:authenticated",
					},
				},
			},
			machine: machine,
			err:     fmt.Errorf("ip address '%v' cannot be associated with node '%s'", "192.0.2.25", machine.Status.NodeRef.Name),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &reconciler{}

			// Parse the certificate request
			csrBlock, rest := pem.Decode(tc.csr.Spec.Request)
			if csrBlock == nil {
				t.Fatal("no certificate request found for the given CSR")
			}
			if len(rest) != 0 {
				t.Fatal("found more than one PEM encoded block in the result")
			}
			certReq, err := x509.ParseCertificateRequest(csrBlock.Bytes)
			if err != nil {
				t.Fatalf("failed to parse x509 certificate request: %v", err)
			}

			err = r.validateX509CSR(tc.csr, certReq, tc.machine)
			if err != nil && err.Error() != tc.err.Error() {
				t.Errorf("expected error '%v', but got '%v'", tc.err, err)
			} else if err == nil && tc.err != nil {
				t.Errorf("expected error '%v'", tc.err)
			}
		})
	}
}
