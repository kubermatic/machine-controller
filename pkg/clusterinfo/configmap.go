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
	"io/ioutil"
	"net"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"
)

const (
	configMapName           = "cluster-info"
	kubernetesEndpointsName = "kubernetes"
	securePortName          = "https"
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
	// secrets, configmaps and endpoints, as that would result in a lot of traffic we don't
	// care about
	kubeClient kubernetes.Interface
}

func (p *KubeconfigProvider) GetKubeconfig(ctx context.Context) (*clientcmdapi.Config, error) {
	cm, err := p.getKubeconfigFromConfigMap(ctx)
	if err != nil {
		klog.V(6).Infof("could not get cluster-info kubeconfig from configmap: %v", err)
		klog.V(6).Info("falling back to retrieval via endpoint")
		return p.buildKubeconfigFromEndpoint(ctx)
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

func (p *KubeconfigProvider) buildKubeconfigFromEndpoint(ctx context.Context) (*clientcmdapi.Config, error) {
	e, err := p.kubeClient.CoreV1().Endpoints(metav1.NamespaceDefault).Get(ctx, kubernetesEndpointsName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint from lister: %w", err)
	}

	if len(e.Subsets) == 0 {
		return nil, errors.New("no subsets in the kubernetes endpoints resource")
	}
	subset := e.Subsets[0]

	if len(subset.Addresses) == 0 {
		return nil, errors.New("no addresses in the first subset of the kubernetes endpoints resource")
	}
	address := subset.Addresses[0]

	ip := net.ParseIP(address.IP)
	if ip == nil {
		return nil, errors.New("could not parse ip from ")
	}

	getSecurePort := func(endpointSubset corev1.EndpointSubset) *corev1.EndpointPort {
		for _, p := range subset.Ports {
			if p.Name == securePortName {
				return &p
			}
		}
		return nil
	}

	port := getSecurePort(subset)
	if port == nil {
		return nil, errors.New("no secure port in the subset")
	}
	url := fmt.Sprintf("https://%s", net.JoinHostPort(ip.String(), strconv.Itoa(int(port.Port))))

	caData, err := getCAData(p.clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get ca data from config: %w", err)
	}

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

func getCAData(config *rest.Config) ([]byte, error) {
	if len(config.TLSClientConfig.CAData) > 0 {
		return config.TLSClientConfig.CAData, nil
	}

	return ioutil.ReadFile(config.TLSClientConfig.CAFile)
}

func (p *KubeconfigProvider) GetBearerToken() string {
	return p.clientConfig.BearerToken
}
