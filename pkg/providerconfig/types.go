package providerconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/golang/glog"
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
	*v1.ObjectReference `json:",inline"`
	Key                 string `json:"key"`
}

type ConfigVarString struct {
	Value     string                  `json:"value,omitempty"`
	ValueFrom GlobalSecretKeySelector `json:"valueFrom,omitempty"`
}

// This type only exists to have the same fields as ConfigVarString but
// not its funcs, so it can be used as target for json.Unmarshal without
// causing a recursion
type configVarStringInheritant struct {
	ConfigVarString `json:",inline"`
}

func (c configVarStringInheritant) Namespace() string {
	return c.ConfigVarString.ValueFrom.Namespace
}

func (c configVarStringInheritant) Name() string {
	return c.ConfigVarString.ValueFrom.Name
}

func (c configVarStringInheritant) Key() string {
	return c.ConfigVarString.ValueFrom.Key
}

func (configVarString *ConfigVarString) UnmarshalJSON(b []byte) error {
	if !bytes.HasPrefix(b, []byte("{")) {
		configVarString = &ConfigVarString{Value: string(b)}
		return nil
	}
	// This type must have the same fields as ConfigVarString but not
	// its UnmarshalJSON, otherwise we cause a recursion
	var cvsDummy configVarStringInheritant
	err := json.Unmarshal(b, &cvsDummy)
	if err != nil {
		return err
	}
	objr := v1.ObjectReference{
		Namespace: cvsDummy.Namespace(),
		Name:      cvsDummy.Name(),
	}
	cvs := ConfigVarString{
		ValueFrom: GlobalSecretKeySelector{
			ObjectReference: &objr,
			Key:             cvsDummy.Key(),
		},
	}
	configVarString = &cvs
	return nil
	//	var cvs map[string]interface{}
	//	err := json.Unmarshal(b, &cvs)
	//	if err != nil {
	//		return err
	//	}
	//	if valueFrom, ok := cvs["valueFrom"]; ok {
	//		if valueFromMap, ok := valueFrom.(map[string]interface{}); ok {
	//			var namespace, name, key string
	//			if ns, ok := valueFromMap["namespace"]; ok {
	//				if nsString, ok := ns.(string); ok {
	//					namespace = nsString
	//				}
	//			}
	//			if n, ok := valueFromMap["name"]; ok {
	//				if nString, ok := n.(string); ok {
	//					name = nString
	//				}
	//			}
	//			if k, ok := valueFromMap["key"]; ok {
	//				if kString, ok := k.(string); ok {
	//					key = kString
	//				}
	//			}
	//			gsks := GlobalSecretKeySelector{v1.ObjectReference{Namespace: namespace, Name: name}, key}
	//			configVarString = &ConfigVarString{ValueFrom: gsks}
	//			return nil
	//
	//		}
	//		return fmt.Errorf("valueFrom must be a string map but is '%s'!", reflect.TypeOf(valueFrom))
	//	}
	//	if value, ok := cvs["value"]; ok {
	//		if valueString, ok := value.(string); ok {
	//			configVarString = &ConfigVarString{Value: valueString}
	//			return nil
	//		}
	//		return fmt.Errorf("value must be a string!")
	//	}
	//	return fmt.Errorf("error decoding, object is neither a string nor a map with a 'value' or 'valueFrom' key!")
}

type ConfigVarBool struct {
	Value     bool                    `json:"value,omitempty"`
	ValueFrom GlobalSecretKeySelector `json:"valueFrom,omitempty"`
}

func (configVarBool *ConfigVarBool) UnmarshalJSON(b []byte) error {
	var bVar bool
	if err := json.Unmarshal(b, &bVar); err == nil {
		configVarBool = &ConfigVarBool{Value: bVar}
		return nil
	}
	var cvb ConfigVarBool
	err := json.Unmarshal(b, &cvb)
	if err != nil {
		return err
	}
	configVarBool = &cvb
	return nil
}

type SecretKeyGetter struct {
	kubeClient kubernetes.Interface
}

func (secretKeyGetter *SecretKeyGetter) GetConfigVarStringValue(configVar ConfigVarString) (string, error) {
	// We need all three of these to fetch and use a secret, so fallback to .Value if any of it is an empty string
	if configVar.ValueFrom.Name == "" || configVar.ValueFrom.Namespace == "" || configVar.ValueFrom.Key == "" {
		glog.V(6).Infof("Not getting secret for configVar, valueFrom values: Name: '%s', Namespace: '%s', Key: '%s'", configVar.ValueFrom.Name, configVar.ValueFrom.Namespace, configVar.ValueFrom.Key)
		return configVar.Value, nil
	}
	glog.V(6).Infof("Getting secret '%s' in namespace '%s' for config var...", configVar.ValueFrom.Name, configVar.ValueFrom.Namespace)
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

func (secretKeyGetter *SecretKeyGetter) GetConfigVarBoolValue(configVar ConfigVarBool) (bool, error) {
	cvs := ConfigVarString{Value: strconv.FormatBool(configVar.Value), ValueFrom: configVar.ValueFrom}
	stringVal, err := secretKeyGetter.GetConfigVarStringValue(cvs)
	if err != nil {
		return false, err
	}
	boolVal, err := strconv.ParseBool(stringVal)
	if err != nil {
		return false, err
	}
	return boolVal, nil
}

func NewSecretKeyGetter(kubeClient kubernetes.Interface) *SecretKeyGetter {
	return &SecretKeyGetter{kubeClient: kubeClient}
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
