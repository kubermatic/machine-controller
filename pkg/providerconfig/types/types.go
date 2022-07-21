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

package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/jsonutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OperatingSystem defines the host operating system.
type OperatingSystem string

const (
	OperatingSystemUbuntu       OperatingSystem = "ubuntu"
	OperatingSystemCentOS       OperatingSystem = "centos"
	OperatingSystemAmazonLinux2 OperatingSystem = "amzn2"
	OperatingSystemSLES         OperatingSystem = "sles"
	OperatingSystemRHEL         OperatingSystem = "rhel"
	OperatingSystemFlatcar      OperatingSystem = "flatcar"
	OperatingSystemRockyLinux   OperatingSystem = "rockylinux"
)

type CloudProvider string

const (
	CloudProviderAWS                 CloudProvider = "aws"
	CloudProviderAzure               CloudProvider = "azure"
	CloudProviderDigitalocean        CloudProvider = "digitalocean"
	CloudProviderGoogle              CloudProvider = "gce"
	CloudProviderEquinixMetal        CloudProvider = "equinixmetal"
	CloudProviderPacket              CloudProvider = "packet"
	CloudProviderHetzner             CloudProvider = "hetzner"
	CloudProviderKubeVirt            CloudProvider = "kubevirt"
	CloudProviderLinode              CloudProvider = "linode"
	CloudProviderNutanix             CloudProvider = "nutanix"
	CloudProviderOpenstack           CloudProvider = "openstack"
	CloudProviderVsphere             CloudProvider = "vsphere"
	CloudProviderVMwareCloudDirector CloudProvider = "vmware-cloud-director"
	CloudProviderFake                CloudProvider = "fake"
	CloudProviderAlibaba             CloudProvider = "alibaba"
	CloudProviderAnexia              CloudProvider = "anexia"
	CloudProviderScaleway            CloudProvider = "scaleway"
	CloudProviderBaremetal           CloudProvider = "baremetal"
	CloudProviderExternal            CloudProvider = "external"
)

var (
	ErrOSNotSupported = errors.New("os not supported")

	// AllOperatingSystems is a slice containing all supported operating system identifiers.
	AllOperatingSystems = []OperatingSystem{
		OperatingSystemUbuntu,
		OperatingSystemCentOS,
		OperatingSystemAmazonLinux2,
		OperatingSystemSLES,
		OperatingSystemRHEL,
		OperatingSystemFlatcar,
		OperatingSystemRockyLinux,
	}

	// AllCloudProviders is a slice containing all supported cloud providers.
	AllCloudProviders = []CloudProvider{
		CloudProviderAWS,
		CloudProviderAzure,
		CloudProviderDigitalocean,
		CloudProviderEquinixMetal,
		CloudProviderPacket,
		CloudProviderGoogle,
		CloudProviderHetzner,
		CloudProviderKubeVirt,
		CloudProviderLinode,
		CloudProviderNutanix,
		CloudProviderOpenstack,
		CloudProviderVsphere,
		CloudProviderVMwareCloudDirector,
		CloudProviderFake,
		CloudProviderAlibaba,
		CloudProviderAnexia,
		CloudProviderScaleway,
		CloudProviderBaremetal,
	}
)

// DNSConfig contains a machine's DNS configuration.
type DNSConfig struct {
	Servers []string `json:"servers"`
}

// NetworkConfig contains a machine's static network configuration.
type NetworkConfig struct {
	CIDR     string        `json:"cidr"`
	Gateway  string        `json:"gateway"`
	DNS      DNSConfig     `json:"dns"`
	IPFamily util.IPFamily `json:"ipFamily,omitempty"`
}

func (n *NetworkConfig) IsStaticIPConfig() bool {
	if n == nil {
		return false
	}
	return n.CIDR != "" ||
		n.Gateway != "" ||
		len(n.DNS.Servers) != 0
}

func (n *NetworkConfig) GetIPFamily() util.IPFamily {
	if n == nil {
		return util.Unspecified
	}
	return n.IPFamily
}

type Config struct {
	SSHPublicKeys []string `json:"sshPublicKeys"`
	CAPublicKey   string   `json:"caPublicKey"`

	CloudProvider     CloudProvider        `json:"cloudProvider"`
	CloudProviderSpec runtime.RawExtension `json:"cloudProviderSpec"`

	OperatingSystem     OperatingSystem      `json:"operatingSystem"`
	OperatingSystemSpec runtime.RawExtension `json:"operatingSystemSpec"`

	// +optional
	Network *NetworkConfig `json:"network,omitempty"`

	// +optional
	OverwriteCloudConfig *string `json:"overwriteCloudConfig,omitempty"`
}

// GlobalObjectKeySelector is needed as we can not use v1.SecretKeySelector
// because it is not cross namespace.
type GlobalObjectKeySelector struct {
	corev1.ObjectReference `json:",inline"`
	Key                    string `json:"key,omitempty"`
}

type GlobalSecretKeySelector GlobalObjectKeySelector
type GlobalConfigMapKeySelector GlobalObjectKeySelector

type ConfigVarString struct {
	Value           string                     `json:"value,omitempty"`
	SecretKeyRef    GlobalSecretKeySelector    `json:"secretKeyRef,omitempty"`
	ConfigMapKeyRef GlobalConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

// This type only exists to have the same fields as ConfigVarString but
// not its funcs, so it can be used as target for json.Unmarshal without
// causing a recursion.
type configVarStringWithoutUnmarshaller ConfigVarString

// MarshalJSON converts a configVarString to its JSON form, omitting empty strings.
// This is done to not have the json object cluttered with empty strings
// This will eventually hopefully be resolved within golang itself
// https://github.com/golang/go/issues/11939.
func (configVarString ConfigVarString) MarshalJSON() ([]byte, error) {
	var secretKeyRefEmpty, configMapKeyRefEmpty bool
	if configVarString.SecretKeyRef.ObjectReference.Namespace == "" &&
		configVarString.SecretKeyRef.ObjectReference.Name == "" &&
		configVarString.SecretKeyRef.Key == "" {
		secretKeyRefEmpty = true
	}

	if configVarString.ConfigMapKeyRef.ObjectReference.Namespace == "" &&
		configVarString.ConfigMapKeyRef.ObjectReference.Name == "" &&
		configVarString.ConfigMapKeyRef.Key == "" {
		configMapKeyRefEmpty = true
	}

	if secretKeyRefEmpty && configMapKeyRefEmpty {
		return []byte(fmt.Sprintf(`"%s"`, configVarString.Value)), nil
	}

	buffer := bytes.NewBufferString("{")
	if !secretKeyRefEmpty {
		jsonVal, err := json.Marshal(configVarString.SecretKeyRef)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(fmt.Sprintf(`"secretKeyRef":%s`, string(jsonVal)))
	}

	if !configMapKeyRefEmpty {
		var leadingComma string
		if !secretKeyRefEmpty {
			leadingComma = ","
		}
		jsonVal, err := json.Marshal(configVarString.ConfigMapKeyRef)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(fmt.Sprintf(`%s"configMapKeyRef":%s`, leadingComma, jsonVal))
	}

	if configVarString.Value != "" {
		buffer.WriteString(fmt.Sprintf(`,"value":"%s"`, configVarString.Value))
	}

	buffer.WriteString("}")
	return buffer.Bytes(), nil
}

func (configVarString *ConfigVarString) UnmarshalJSON(b []byte) error {
	if !bytes.HasPrefix(b, []byte("{")) {
		b = bytes.TrimPrefix(b, []byte(`"`))
		b = bytes.TrimSuffix(b, []byte(`"`))

		// `Unquote` expects the input string to be inside quotation marks.
		//  Since we can have a string without any quotations, in which case `TrimPrefix` and
		// `TrimSuffix` will be noop. We explicitly add quotation marks to the input string
		// to make sure that `Unquote` never fails.
		s, err := strconv.Unquote("\"" + string(b) + "\"")
		if err != nil {
			return err
		}
		configVarString.Value = s
		return nil
	}
	// This type must have the same fields as ConfigVarString but not
	// its UnmarshalJSON, otherwise we cause a recursion
	var cvsDummy configVarStringWithoutUnmarshaller
	err := json.Unmarshal(b, &cvsDummy)
	if err != nil {
		return err
	}
	configVarString.Value = cvsDummy.Value
	configVarString.SecretKeyRef = cvsDummy.SecretKeyRef
	configVarString.ConfigMapKeyRef = cvsDummy.ConfigMapKeyRef
	return nil
}

type ConfigVarBool struct {
	Value           *bool                      `json:"value,omitempty"`
	SecretKeyRef    GlobalSecretKeySelector    `json:"secretKeyRef,omitempty"`
	ConfigMapKeyRef GlobalConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

type configVarBoolWithoutUnmarshaller ConfigVarBool

// MarshalJSON encodes the configVarBool, omitting empty strings
// This is done to not have the json object cluttered with empty strings
// This will eventually hopefully be resolved within golang itself
// https://github.com/golang/go/issues/11939
func (configVarBool ConfigVarBool) MarshalJSON() ([]byte, error) {
	var secretKeyRefEmpty, configMapKeyRefEmpty bool
	if configVarBool.SecretKeyRef.ObjectReference.Namespace == "" &&
		configVarBool.SecretKeyRef.ObjectReference.Name == "" &&
		configVarBool.SecretKeyRef.Key == "" {
		secretKeyRefEmpty = true
	}

	if configVarBool.ConfigMapKeyRef.ObjectReference.Namespace == "" &&
		configVarBool.ConfigMapKeyRef.ObjectReference.Name == "" &&
		configVarBool.ConfigMapKeyRef.Key == "" {
		configMapKeyRefEmpty = true
	}

	if secretKeyRefEmpty && configMapKeyRefEmpty {
		jsonVal, err := json.Marshal(configVarBool.Value)
		if err != nil {
			return []byte{}, err
		}
		return jsonVal, nil
	}

	buffer := bytes.NewBufferString("{")
	if !secretKeyRefEmpty {
		jsonVal, err := json.Marshal(configVarBool.SecretKeyRef)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(fmt.Sprintf(`"secretKeyRef":%s`, string(jsonVal)))
	}

	if !configMapKeyRefEmpty {
		var leadingComma string
		if !secretKeyRefEmpty {
			leadingComma = ","
		}
		jsonVal, err := json.Marshal(configVarBool.ConfigMapKeyRef)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(fmt.Sprintf(`%s"configMapKeyRef":%s`, leadingComma, jsonVal))
	}

	if configVarBool.Value != nil {
		jsonVal, err := json.Marshal(configVarBool.Value)
		if err != nil {
			return []byte{}, err
		}

		buffer.WriteString(fmt.Sprintf(`,"value":%v`, string(jsonVal)))
	}

	buffer.WriteString("}")

	return buffer.Bytes(), nil
}

func (configVarBool *ConfigVarBool) UnmarshalJSON(b []byte) error {
	if !bytes.HasPrefix(b, []byte("{")) {
		var val *bool
		if err := json.Unmarshal(b, &val); err != nil {
			return fmt.Errorf("Error parsing value: '%w'", err)
		}
		configVarBool.Value = val

		return nil
	}

	var cvbDummy configVarBoolWithoutUnmarshaller

	err := json.Unmarshal(b, &cvbDummy)
	if err != nil {
		return err
	}

	configVarBool.Value = cvbDummy.Value
	configVarBool.SecretKeyRef = cvbDummy.SecretKeyRef
	configVarBool.ConfigMapKeyRef = cvbDummy.ConfigMapKeyRef

	return nil
}

func GetConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, error) {
	if provSpec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}

	var cfg Config

	if len(provSpec.Value.Raw) == 0 {
		return &cfg, nil
	}

	if err := jsonutil.StrictUnmarshal(provSpec.Value.Raw, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
