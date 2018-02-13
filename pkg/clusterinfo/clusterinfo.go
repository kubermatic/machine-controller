package clusterinfo

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// GetFromKubeconfig gets cluster information stored under a ConfigMap
func GetFromKubeconfig(cfgMapLister listerscorev1.ConfigMapLister) (*clientcmdapi.Config, error) {
	cm, err := cfgMapLister.ConfigMaps(metav1.NamespacePublic).Get("cluster-info")
	if err != nil {
		return nil, err
	}
	data, found := cm.Data["kubeconfig"]
	if !found {
		return nil, fmt.Errorf("no kubeconfig found in cluster-info configmap")
	}
	return clientcmd.Load([]byte(data))
}
