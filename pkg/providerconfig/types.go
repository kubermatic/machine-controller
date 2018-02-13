package providerconfig

import (
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/api/core/v1"
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

	CloudProvider     CloudProvider        `json:"cloudProvider,omitempty"`
	CloudProviderSpec runtime.RawExtension `json:"cloudProviderSpec,omitempty"`

	OperatingSystem     OperatingSystem      `json:"operatingSystem"`
	OperatingSystemSpec runtime.RawExtension `json:"operatingSystemSpec"`
}

// We can not use v1.SecretKeySelector because it is not cross namespace
type GlobalSecretKeySelector struct {
	v1.ObjectReference `json:",inline"`
	Key                string `json:"key"`
}

type ConfigVarString struct {
	Value     string                  `json:"value,omitempty"`
	ValueFrom GlobalSecretKeySelector `json:"valueFrom,omitempty"`
}

type ConfigVarBool struct {
	Value     bool                    `json:"value,omitempty"`
	ValueFrom GlobalSecretKeySelector `json:"valueFrom,omitempty"`
}

type SecretKeyGetter struct {
	kubeClient kubernetes.Interface
}

func (secretKeyGetter *SecretKeyGetter) getConfigVarStringValue(kubeClient kubernetes.Interface, configVar ConfigVarString) (string, error) {
	if configVar.Value != "" {
		return configVar.Value, nil
	}
	secret, err := secretKeyGetter.kubeClient.CoreV1().Secrets(
		configVar.ValueFrom.Namespace).Get(configVar.ValueFrom.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error retrieving secret '%s' from namespace '%s': '%v'", configVar.ValueFrom.Name, configVar.ValueFrom.Namespace, err)
	}
	if val, ok := secret.Data[configVar.ValueFrom.Key]; ok {
		return string(val), nil
	}
	return "", fmt.Errorf("secret '%s' in namespace '%s' has no key '%s'!", configVar.ValueFrom.Name, configVar.ValueFrom.Namespace, configVar.ValueFrom.Key)
}

func GetConfig(r runtime.RawExtension) (*Config, error) {
	p := new(Config)
	if len(r.Raw) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(r.Raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
