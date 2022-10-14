/*
Copyright 2022 The Machine Controller Authors.

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

package provisioner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

const (
	maxRetrieForMachines = 5
	hostnameAnnotation   = "ssh-username"

	userDataTemplate = `#cloud-config
ssh_pwauth: false

{{- if .ProviderSpec.SSHPublicKeys }}
ssh_authorized_keys:
{{- range .ProviderSpec.SSHPublicKeys }}
- "{{ . }}"
{{- end }}
{{- end }}
`
)

func getUserData(pconfig *providerconfigtypes.Config) (string, error) {
	data := struct {
		ProviderSpec *providerconfigtypes.Config
	}{
		ProviderSpec: pconfig,
	}

	tmpl, err := template.New("user-data").Funcs(userdatahelper.TxtFuncMap()).Parse(userDataTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %w", err)
	}

	return userdatahelper.CleanupTemplateOutput(buf.String())
}

type MachineInstance struct {
	inst    instance.Instance
	sshUser string
}

func CreateMachines(ctx context.Context, machines []clusterv1alpha1.Machine) (*output, error) {
	providerData := &cloudprovidertypes.ProviderData{
		Ctx:             ctx,
		ProvisionerMode: true,
	}

	var instances []MachineInstance

	// TODO: Dump all the errors in an array and do the max that is possible without early exit
	for _, machine := range machines {
		prov, err := getProvider(ctx, machine)
		if err != nil {
			return nil, err
		}

		machineCreated := false
		providerInstance, err := prov.Get(ctx, &machine, providerData)
		if err != nil {
			// case 1: instance was not found and we are going to create one
			if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
				// Get userdata (needed to inject SSH keys to instances)
				pconfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
				if err != nil {
					return nil, fmt.Errorf("failed to get providerSpec: %w", err)
				}
				userdata, err := getUserData(pconfig)
				if err != nil {
					return nil, err
				}

				// Create the instance
				_, err = prov.Create(ctx, &machine, providerData, userdata)
				if err != nil {
					return nil, err
				}
				machineCreated = true
			} else if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
				// case 2: terminal error was returned and manual interaction is required to recover
				return nil, fmt.Errorf("failed to create machine at cloudprovider, due to %w", err)
			} else {
				// case 3: transient error was returned, requeue the request and try again in the future
				return nil, fmt.Errorf("failed to get instance from provider: %w", err)
			}
		}

		if machineCreated {
			for i := 0; i < maxRetrieForMachines; i++ {
				providerInstance, err = prov.Get(ctx, &machine, providerData)
				if err != nil {
					return nil, err
				}

				addresses := providerInstance.Addresses()
				if len(addresses) > 0 && publicAndPrivateIPExist(addresses) {
					break
				}
				logrus.Info("Waiting 5 seconds for machine address assignment.")
				time.Sleep(5 * time.Second)
			}
		}

		// Instance exists
		addresses := providerInstance.Addresses()
		if len(addresses) == 0 {
			return nil, fmt.Errorf("machine %s has not been assigned an IP yet", providerInstance.Name())
		}

		if machineCreated {
			logrus.Infof("Machine %q was successfully created.", providerInstance.Name())
		} else {
			logrus.Infof("Machine %q already exists.", providerInstance.Name())
		}

		sshUser := "root"
		if machine.Annotations != nil {
			if user := machine.Annotations[hostnameAnnotation]; sshUser != "" {
				sshUser = user
			}
		}

		machineInstance := MachineInstance{
			inst:    providerInstance,
			sshUser: sshUser,
		}

		instances = append(instances, machineInstance)
	}

	output := getMachineProvisionerOutput(instances)
	return &output, nil
}

func getProvider(ctx context.Context, machine clusterv1alpha1.Machine) (cloudprovidertypes.Provider, error) {
	providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}
	skg := providerconfig.NewConfigVarResolver(ctx, nil)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider %q: %w", providerConfig.CloudProvider, err)
	}

	return prov, nil
}
