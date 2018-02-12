package providerconfig

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

var (
	ErrOSNotSupported = errors.New("os not supported")
)

type OperatingSystem string

const (
	OperatingSystemCoreos OperatingSystem = "coreos"
	OperatingSystemUbuntu OperatingSystem = "ubuntu"
)

type CloudProvider string

const (
	CloudProviderAWS          CloudProvider = "aws"
	CloudProviderDigitalocean CloudProvider = "digitalocean"
	CloudProviderOpenstack    CloudProvider = "openstack"
	CloudProviderHetzner      CloudProvider = "hetzner"
)

type Config struct {
	SSHPublicKeys []string `json:"sshPublicKeys"`

	CloudProvider           CloudProvider        `json:"cloudProvider,omitempty"`
	CloudProviderSpec       runtime.RawExtension `json:"cloudProviderSpec,omitempty"`
	CloudProviderSecretName string               `json:"cloudProviderSecretName"`

	OperatingSystem     OperatingSystem      `json:"operatingSystem"`
	OperatingSystemSpec runtime.RawExtension `json:"operatingSystemSpec"`
}

func GetConfig(kubeClient kubernetes.Interface, r runtime.RawExtension, includeCloudProviderConfig bool) (*Config, error) {
	p := new(Config)
	if len(r.Raw) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(r.Raw, p); err != nil {
		return nil, err
	}

	if includeCloudProviderConfig && p.CloudProviderSecretName != "" {
		secret, err := kubeClient.CoreV1().Secrets(metav1.NamespaceSystem).Get(p.CloudProviderSecretName, metav1.GetOptions{})
		if err != nil {
			return p, fmt.Errorf("error retrieving cloudProviderSecret: '%v'", err)
		}

		if cloudProvider, ok := secret.Data["cloudProvider"]; ok {
			p.CloudProvider = CloudProvider(cloudProvider)
			glog.V(2).Infof("Cloudprovider was set from secret '%s'", p.CloudProviderSecretName)
		}

		if cloudProviderSpec, ok := secret.Data["cloudProviderSpec"]; ok {
			p.CloudProviderSpec = runtime.RawExtension{Raw: []byte(cloudProviderSpec)}
			glog.V(2).Infof("cloudProviderSpec was set from secret '%s'", p.CloudProviderSecretName)
		}

		return p, nil

	} else {
		glog.V(2).Infof("Not retrieving secret '%s'", p.CloudProviderSecretName)
	}
	return p, nil
}
