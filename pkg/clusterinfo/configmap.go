package clusterinfo

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	configMapName           = "cluster-info"
	kubernetesEndpointsName = "kubernetes"
	securePortName          = "https"
)

func New(clientConfig *rest.Config, configMapLister listerscorev1.ConfigMapLister, endpointLister listerscorev1.EndpointsLister) *KubeconfigProvider {
	return &KubeconfigProvider{
		configMapLister: configMapLister,
		endpointLister:  endpointLister,
		clientConfig:    clientConfig,
	}
}

type KubeconfigProvider struct {
	clientConfig    *rest.Config
	configMapLister listerscorev1.ConfigMapLister
	endpointLister  listerscorev1.EndpointsLister
}

func (p *KubeconfigProvider) GetKubeconfig() (*clientcmdapi.Config, error) {
	cm, err := p.getKubeconfigFromConfigMap()
	if err != nil {
		glog.V(6).Infof("could not get cluster-info kubeconfig from configmap: %v", err)
		glog.V(6).Info("falling back to retrieval via endpoint")
		return p.buildKubeconfigFromEndpoint()
	}
	return cm, nil
}

func (p *KubeconfigProvider) getKubeconfigFromConfigMap() (*clientcmdapi.Config, error) {
	cm, err := p.configMapLister.ConfigMaps(metav1.NamespacePublic).Get(configMapName)
	if err != nil {
		return nil, err
	}
	data, found := cm.Data["kubeconfig"]
	if !found {
		return nil, errors.New("no kubeconfig found in cluster-info configmap")
	}
	return clientcmd.Load([]byte(data))
}

func (p *KubeconfigProvider) buildKubeconfigFromEndpoint() (*clientcmdapi.Config, error) {
	e, err := p.endpointLister.Endpoints(metav1.NamespaceDefault).Get(kubernetesEndpointsName)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint from lister: %v", err)
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
	url := fmt.Sprintf("https://%s:%d", ip.String(), port.Port)

	caData, err := getCAData(p.clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get ca data from config: %v", err)
	}

	return &clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*clientcmdapi.Cluster{
			"": {
				Server: url,
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
