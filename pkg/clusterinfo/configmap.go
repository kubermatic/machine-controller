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
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"go.uber.org/zap"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	configMapName     = "cluster-info"
	kubernetesService = "kubernetes"
	securePortName    = "https"
)

func New(clientConfig *rest.Config, kubeClient kubernetes.Interface) *KubeconfigProvider {
	return &KubeconfigProvider{
		clientConfig: clientConfig,
		kubeClient:   kubeClient,
	}
}

type KubeconfigProvider struct {
	clientConfig *rest.Config
	// We use a kubeClient to not accidentally create listers in the ctrlruntimeclient for
	// secrets, configmaps and endpointslices, as that would result in a lot of traffic we don't
	// care about
	kubeClient kubernetes.Interface
}

func (p *KubeconfigProvider) GetKubeconfig(ctx context.Context, log *zap.SugaredLogger) (*clientcmdapi.Config, error) {
	cm, err := p.getKubeconfigFromConfigMap(ctx)
	if err != nil {
		log.Debugw("Failed to get cluster-info kubeconfig from configmap; falling back to retrieval via endpointslice", zap.Error(err))
		return p.buildKubeconfigFromEndpointSlice(ctx)
	}
	return cm, nil
}

func (p *KubeconfigProvider) getKubeconfigFromConfigMap(ctx context.Context) (*clientcmdapi.Config, error) {
	cm, err := p.kubeClient.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data, found := cm.Data["kubeconfig"]
	if !found {
		return nil, errors.New("no kubeconfig found in cluster-info configmap")
	}
	return clientcmd.Load([]byte(data))
}

func (p *KubeconfigProvider) buildKubeconfigFromEndpointSlice(ctx context.Context) (*clientcmdapi.Config, error) {
	slices, err := p.kubeClient.DiscoveryV1().EndpointSlices(metav1.NamespaceDefault).List(ctx,
		metav1.ListOptions{LabelSelector: discoveryv1.LabelServiceName + "=" + kubernetesService})
	if err != nil {
		return nil, fmt.Errorf("failed to list endpointslices: %w", err)
	}

	if len(slices.Items) == 0 {
		return nil, errors.New("no endpointslices found for kubernetes service")
	}

	caData, err := getCAData(p.clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get ca data from config: %w", err)
	}

	for _, slice := range slices.Items {
		port := getSecurePortFromSlice(slice.Ports)
		if port == nil {
			continue
		}

		// Find a ready endpoint with a valid address
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || !*endpoint.Conditions.Ready {
				continue
			}

			if len(endpoint.Addresses) == 0 {
				continue
			}

			ip := net.ParseIP(endpoint.Addresses[0])
			if ip == nil {
				continue
			}

			url := fmt.Sprintf("https://%s", net.JoinHostPort(ip.String(), strconv.Itoa(int(*port.Port))))

			return &clientcmdapi.Config{
				Kind:       "Config",
				APIVersion: "v1",
				Clusters: map[string]*clientcmdapi.Cluster{
					"": {
						Server:                   url,
						CertificateAuthorityData: caData,
					},
				},
			}, nil
		}
	}

	return nil, errors.New("no ready endpoint found in kubernetes endpointslices")
}

func getSecurePortFromSlice(ports []discoveryv1.EndpointPort) *discoveryv1.EndpointPort {
	for _, p := range ports {
		if p.Name != nil && *p.Name == securePortName && p.Port != nil {
			return &p
		}
	}
	return nil
}

func getCAData(config *rest.Config) ([]byte, error) {
	if len(config.CAData) > 0 {
		return config.CAData, nil
	}

	return os.ReadFile(config.CAFile)
}

func (p *KubeconfigProvider) GetBearerToken() string {
	return p.clientConfig.BearerToken
}
