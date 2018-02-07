package utils

import (
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TODO: desc
func GetClusterInfoKubeconfig(cfgMapLister listerscorev1.ConfigMapLister) (*clientcmdapi.Config, error) {
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

// TODO: desc
func ReadinessChecks(cfgMapLister listerscorev1.ConfigMapLister) map[string]healthcheck.Check {
	return map[string]healthcheck.Check{
		"valid-info-kubeconfig": func() error {
			cm, err := GetClusterInfoKubeconfig(cfgMapLister)
			if err != nil {
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("invalid kubeconfig: no clusters found")
				glog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
}
