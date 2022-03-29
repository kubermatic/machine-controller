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

package providerconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/amzn2"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"
	"github.com/kubermatic/machine-controller/pkg/userdata/rhel"
	"github.com/kubermatic/machine-controller/pkg/userdata/sles"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigVarResolver struct {
	ctx    context.Context
	client ctrlruntimeclient.Client
}

func (cvr *ConfigVarResolver) GetConfigVarDurationValue(configVar *providerconfigtypes.ConfigVarString) (time.Duration, error) {
	durStr, err := cvr.GetConfigVarStringValue(configVar)
	if err != nil {
		return 0, err
	}

	return time.ParseDuration(durStr)
}

func (cvr *ConfigVarResolver) GetConfigVarDurationValueOrDefault(configVar *providerconfigtypes.ConfigVarString, defaultDuration time.Duration) (time.Duration, error) {
	durStr, err := cvr.GetConfigVarStringValue(configVar)
	if err != nil {
		return 0, err
	}

	if durStr == "" {
		return defaultDuration, nil
	}

	duration, err := time.ParseDuration(durStr)
	if err != nil {
		return 0, err
	}

	if duration <= 0 {
		return defaultDuration, nil
	}

	return duration, nil
}

func (cvr *ConfigVarResolver) GetConfigVarStringValue(configVar *providerconfigtypes.ConfigVarString) (string, error) {
	if configVar == nil {
		return "", nil
	}
	// We need all three of these to fetch and use a secret
	if configVar.SecretKeyRef.Name != "" && configVar.SecretKeyRef.Namespace != "" && configVar.SecretKeyRef.Key != "" {
		secret := &corev1.Secret{}
		name := types.NamespacedName{Namespace: configVar.SecretKeyRef.Namespace, Name: configVar.SecretKeyRef.Name}
		if err := cvr.client.Get(cvr.ctx, name, secret); err != nil {
			return "", fmt.Errorf("error retrieving secret '%s' from namespace '%s': '%v'", configVar.SecretKeyRef.Name, configVar.SecretKeyRef.Namespace, err)
		}
		if val, ok := secret.Data[configVar.SecretKeyRef.Key]; ok {
			return string(val), nil
		}
		return "", fmt.Errorf("secret '%s' in namespace '%s' has no key '%s'", configVar.SecretKeyRef.Name, configVar.SecretKeyRef.Namespace, configVar.SecretKeyRef.Key)
	}

	// We need all three of these to fetch and use a configmap
	if configVar.ConfigMapKeyRef.Name != "" && configVar.ConfigMapKeyRef.Namespace != "" && configVar.ConfigMapKeyRef.Key != "" {
		configMap := &corev1.ConfigMap{}
		name := types.NamespacedName{Namespace: configVar.ConfigMapKeyRef.Namespace, Name: configVar.ConfigMapKeyRef.Name}
		if err := cvr.client.Get(cvr.ctx, name, configMap); err != nil {
			return "", fmt.Errorf("error retrieving configmap '%s' from namespace '%s': '%v'", configVar.ConfigMapKeyRef.Name, configVar.ConfigMapKeyRef.Namespace, err)
		}
		if val, ok := configMap.Data[configVar.ConfigMapKeyRef.Key]; ok {
			return val, nil
		}
		return "", fmt.Errorf("configmap '%s' in namespace '%s' has no key '%s'", configVar.ConfigMapKeyRef.Name, configVar.ConfigMapKeyRef.Namespace, configVar.ConfigMapKeyRef.Key)
	}

	return configVar.Value, nil
}

// GetConfigVarStringValueOrEnv tries to get the value from ConfigVarString, when it fails, it falls back to
// getting the value from an environment variable specified by envVarName parameter
func (cvr *ConfigVarResolver) GetConfigVarStringValueOrEnv(configVar *providerconfigtypes.ConfigVarString, envVarName string) (string, error) {
	if configVar != nil {
		cfgVar, err := cvr.GetConfigVarStringValue(configVar)
		if err == nil && len(cfgVar) > 0 {
			return cfgVar, err
		}
	}

	envVal, _ := os.LookupEnv(envVarName)
	return envVal, nil
}

func (cvr *ConfigVarResolver) GetConfigVarBoolValue(configVar *providerconfigtypes.ConfigVarBool) (bool, error) {
	if configVar == nil {
		return false, nil
	}
	cvs := &providerconfigtypes.ConfigVarString{Value: strconv.FormatBool(configVar.Value), SecretKeyRef: configVar.SecretKeyRef}
	stringVal, err := cvr.GetConfigVarStringValue(cvs)
	if err != nil {
		return false, err
	}
	boolVal, err := strconv.ParseBool(stringVal)
	if err != nil {
		return false, err
	}
	return boolVal, nil
}

func (cvr *ConfigVarResolver) GetConfigVarBoolValueOrEnv(configVar *providerconfigtypes.ConfigVarBool, envVarName string) (bool, error) {
	if configVar == nil {
		cvs := &providerconfigtypes.ConfigVarString{Value: strconv.FormatBool(configVar.Value), SecretKeyRef: configVar.SecretKeyRef}
		stringVal, err := cvr.GetConfigVarStringValue(cvs)
		if err != nil {
			return false, err
		}
		return strconv.ParseBool(stringVal)
	}
	val, envValFound := os.LookupEnv(envVarName)
	if !envValFound {
		return false, fmt.Errorf("all mechanisms(value, secret, configMap) of getting the value failed, including reading from environment variable = %s which was not set", envVarName)
	}
	return strconv.ParseBool(val)
}

func NewConfigVarResolver(ctx context.Context, client ctrlruntimeclient.Client) *ConfigVarResolver {
	return &ConfigVarResolver{
		ctx:    ctx,
		client: client,
	}
}

func DefaultOperatingSystemSpec(osys providerconfigtypes.OperatingSystem, operatingSystemSpec runtime.RawExtension) (runtime.RawExtension, error) {
	switch osys {
	case providerconfigtypes.OperatingSystemAmazonLinux2:
		return amzn2.DefaultConfig(operatingSystemSpec), nil
	case providerconfigtypes.OperatingSystemCentOS:
		return centos.DefaultConfig(operatingSystemSpec), nil
	case providerconfigtypes.OperatingSystemFlatcar:
		return flatcar.DefaultConfig(operatingSystemSpec), nil
	case providerconfigtypes.OperatingSystemRHEL:
		return rhel.DefaultConfig(operatingSystemSpec), nil
	case providerconfigtypes.OperatingSystemSLES:
		return sles.DefaultConfig(operatingSystemSpec), nil
	case providerconfigtypes.OperatingSystemUbuntu:
		return ubuntu.DefaultConfig(operatingSystemSpec), nil
	}

	return operatingSystemSpec, errors.New("unknown OperatingSystem")
}
