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

package userdata

import (
	"errors"

	"k8c.io/machine-controller/sdk/providerconfig"
	"k8c.io/machine-controller/sdk/userdata/amzn2"
	"k8c.io/machine-controller/sdk/userdata/flatcar"
	"k8c.io/machine-controller/sdk/userdata/rhel"
	"k8c.io/machine-controller/sdk/userdata/rockylinux"
	"k8c.io/machine-controller/sdk/userdata/ubuntu"

	"k8s.io/apimachinery/pkg/runtime"
)

func DefaultOperatingSystemSpec(os providerconfig.OperatingSystem, operatingSystemSpec runtime.RawExtension) (runtime.RawExtension, error) {
	switch os {
	case providerconfig.OperatingSystemAmazonLinux2:
		return amzn2.DefaultConfig(operatingSystemSpec), nil
	case providerconfig.OperatingSystemFlatcar:
		return flatcar.DefaultConfig(operatingSystemSpec), nil
	case providerconfig.OperatingSystemRHEL:
		return rhel.DefaultConfig(operatingSystemSpec), nil
	case providerconfig.OperatingSystemUbuntu:
		return ubuntu.DefaultConfig(operatingSystemSpec), nil
	case providerconfig.OperatingSystemRockyLinux:
		return rockylinux.DefaultConfig(operatingSystemSpec), nil
	}

	return operatingSystemSpec, errors.New("unknown OperatingSystem")
}
