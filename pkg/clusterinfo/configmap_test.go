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

package clusterinfo

import (
	"context"
	"testing"

	"github.com/go-test/deep"
	"github.com/pmezard/go-difflib/difflib"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

var (
	clusterInfoKubeconfig1 = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURHRENDQWdDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREE5TVRzd09RWURWUVFERXpKeWIyOTAKTFdOaExuUTNjV3R4ZURWeGRDNWxkWEp2Y0dVdGQyVnpkRE10WXk1a1pYWXVhM1ZpWlhKdFlYUnBZeTVwYnpBZQpGdzB4T0RBeU1ERXhNelUyTURoYUZ3MHlPREF4TXpBeE16VTJNRGhhTUQweE96QTVCZ05WQkFNVE1uSnZiM1F0ClkyRXVkRGR4YTNGNE5YRjBMbVYxY205d1pTMTNaWE4wTXkxakxtUmxkaTVyZFdKbGNtMWhkR2xqTG1sdk1JSUIKSWpBTkJna3Foa2lHOXcwQkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQXA2SDZWNTZiWUh2Q2V6TGtyZkl6TTgxYgppbzcvWmF3L0xLRXcwZUYrTE12NEUrL1EvZkZoc0hDK21oZUxnMUhXVVBGUFJrNFBRODVtQS80dGppbWpTUEZECms2U0ltektGTFlRZ3dDZ2dpVzhOMmhPKzl6ckJVQUxKRkdCNjRvT2NiQmo2RXIvK05sUEdJM1JSV1dkaUVUV0YKV1lDNGpmSmpiRjVQYnl5WEhuc0dmdFNOWVpCTDcxVzdoOWpMV3B5VVdLTDZaWUFOd0RPTjJSYnA3dHB1dzBYNgprayswQVZ3VnprMzArTU56bWY1MHF3K284MThiZkxVRGthTk1mTFM2STB3UW03UkdnK01nVlJEeTNDdVlxZklXClkyeng2YzdQcXpGc1ZWZklyYTBiMHFhdE5sMVhIajh0K0dOcWRiaTIvRlFqQ3hpbFROdW50VDN2eTJlT0hRSUQKQVFBQm95TXdJVEFPQmdOVkhROEJBZjhFQkFNQ0FxUXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QU5CZ2txaGtpRwo5dzBCQVFzRkFBT0NBUUVBSW1FbklYVjNEeW1DcTlxUDdwK3VKNTV1Zlhka1IyZ2hEVVlyVFRjUHdqUjJqVEhhCmlaQStnOG42UXJVb0NENnN6RytsaGFsN2hQNnhkV3VSalhGSE83Yk52NjNJcUVHelJEZ3c1Z3djcVVUWkV2d2cKZ216NzU5dy9hRmYxVjEyaDFhZlBtQTlFRzVOZEh4c3g5QWxIK0Y2dHlzcHBXaFU4WEVRVUFLQ1BqbndVbUs0cAo3Z3ZUWnIyeno0bndoWm8zTDg5MDNxcHRjcTFsWjRPWXNEb1hvbDF1emFRSDgyeHl3ZVNLQ0tYcE9iaXplNVowCndwbmpkRHVIODd4NHI0TGpNWnB1M3ZYNkxqQkRNUFdrSEhQTjVBaW0xSkx0Ny9STFBnVHRqc0pNclRBUzdoZ1oKZktMTDlRTVFsNnMxckhKNEtrL2U3S0c4SEE0aEVORWhrOVlEZlE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    server: https://foo.bar:6443
  name: ""
contexts: null
current-context: ""
kind: Config
users: null
`
	clusterInfoKubeconfig2 = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: Zm9v
    server: https://192.168.1.2:8443
  name: ""
contexts: null
current-context: ""
kind: Config
users: null
`
)

func TestKubeconfigProvider_GetKubeconfig(t *testing.T) {
	tests := []struct {
		name         string
		objects      []runtime.Object
		clientConfig *rest.Config
		err          error
		resConfig    string
	}{
		{
			name: "successful from configmap",
			objects: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-info",
					Namespace: "kube-public",
				},
				Data: map[string]string{"kubeconfig": clusterInfoKubeconfig1},
			}},
			clientConfig: nil,
			err:          nil,
			resConfig:    clusterInfoKubeconfig1,
		},
		{
			name: "successful from in-cluster via endpointslice - clusterIP",
			objects: []runtime.Object{&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernetes-abc",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "kubernetes",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"192.168.1.2"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
				Ports: []discoveryv1.EndpointPort{
					{
						Name:     ptr.To("https"),
						Port:     ptr.To(int32(8443)),
						Protocol: ptr.To(corev1.ProtocolTCP),
					},
				},
			}},
			clientConfig: &rest.Config{
				TLSClientConfig: rest.TLSClientConfig{
					CAData: []byte(
						"foo",
					),
				},
			},
			err:       nil,
			resConfig: clusterInfoKubeconfig2,
		},
		{
			name: "skips not-ready endpoint and uses ready one",
			objects: []runtime.Object{&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernetes-abc",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "kubernetes",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						// Not-ready endpoint should be skipped
						Addresses: []string{"192.168.1.99"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(false),
						},
					},
					{
						// Ready endpoint should be used
						Addresses: []string{"192.168.1.2"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
				Ports: []discoveryv1.EndpointPort{
					{
						Name:     ptr.To("https"),
						Port:     ptr.To(int32(8443)),
						Protocol: ptr.To(corev1.ProtocolTCP),
					},
				},
			}},
			clientConfig: &rest.Config{
				TLSClientConfig: rest.TLSClientConfig{
					CAData: []byte(
						"foo",
					),
				},
			},
			err:       nil,
			resConfig: clusterInfoKubeconfig2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			client := fake.NewSimpleClientset(test.objects...)

			provider := KubeconfigProvider{
				clientConfig: test.clientConfig,
				kubeClient:   client,
			}

			resConfig, err := provider.GetKubeconfig(ctx, zap.NewNop().Sugar())
			if diff := deep.Equal(err, test.err); diff != nil {
				t.Error(diff)
			}
			if test.err != nil {
				return
			}

			out, err := clientcmd.Write(*resConfig)
			if err != nil {
				t.Error(err)
			}

			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(test.resConfig),
				B:        difflib.SplitLines(string(out)),
				FromFile: "Expected",
				ToFile:   "Got",
				Context:  3,
			}
			diffStr, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Fatal(err)
			}

			if len(diffStr) > 0 {
				t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
			}
		})
	}
}
