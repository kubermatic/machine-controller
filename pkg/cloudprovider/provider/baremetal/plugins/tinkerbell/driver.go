/*
Copyright 2021 The Machine Controller Authors.

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

package tinkerbell

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/aws/smithy-go/ptr"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"go.uber.org/zap"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client"
	tinktypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/types"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/scheme"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type driver struct {
	ClusterName string
	OSImageURL  string

	HardwareRef types.NamespacedName

	TinkClient     ctrlruntimeclient.Client
	HardwareClient client.HardwareClient
	WorkflowClient client.WorkflowClient
	TemplateClient client.TemplateClient
}

func init() {
	// Ensure the Tinkerbell API types are registered with the global scheme.
	if err := tinkv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		panic(fmt.Sprintf("failed to add tinkv1alpha1 to scheme: %v", err))
	}
}

// NewTinkerbellDriver returns a new TinkerBell driver with a configured tinkserver address and a client timeout.
func NewTinkerbellDriver(tinkConfig tinktypes.Config, tinkSpec *tinktypes.TinkerbellPluginSpec) (plugins.PluginDriver, error) {
	tinkClient, err := ctrlruntimeclient.New(tinkConfig.RestConfig, ctrlruntimeclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}
	hwClient := client.NewHardwareClient(tinkClient)

	wkClient := client.NewWorkflowClient(tinkClient)

	tmplClient := client.NewTemplateClient(tinkClient)

	d := driver{
		ClusterName:    tinkSpec.ClusterName.Value,
		TinkClient:     tinkClient,
		HardwareRef:    tinkSpec.HardwareRef,
		HardwareClient: *hwClient,
		WorkflowClient: *wkClient,
		TemplateClient: *tmplClient,
		OSImageURL:     tinkSpec.OSImageURL.Value,
	}

	return &d, nil
}

func (d *driver) GetServer(ctx context.Context) (plugins.Server, error) {
	targetHardware, err := d.HardwareClient.GetHardware(ctx, d.HardwareRef)
	if err != nil {
		return nil, err
	}

	if targetHardware.Spec.Metadata == nil || targetHardware.Spec.Metadata.State == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	server := tinktypes.Hardware{Hardware: targetHardware}
	return &server, nil
}

func (d *driver) ProvisionServer(ctx context.Context, _ *zap.SugaredLogger, meta metav1.ObjectMeta, _ runtime.RawExtension, userdata string) (plugins.Server, error) {
	// Get the hardware object from tinkerbell
	hardware, err := d.HardwareClient.GetHardware(ctx, d.HardwareRef)
	if err != nil {
		return nil, err
	}

	var allowProvision bool
	for _, iface := range hardware.Spec.Interfaces {
		if iface.Netboot != nil && iface.Netboot.AllowPXE != nil && iface.Netboot.AllowPXE == ptr.Bool(false) {
			continue
		}

		if iface.Netboot != nil && iface.Netboot.AllowWorkflow != nil && iface.Netboot.AllowWorkflow == ptr.Bool(false) {
			continue
		}

		allowProvision = true
	}

	if !allowProvision {
		return nil, fmt.Errorf("server %s is not allowed to be provisioned; either hardware allowPXE or allowWorkflow is set to false", hardware.Name)
	}

	// Create template if it doesn't exist
	err = d.TemplateClient.CreateTemplate(ctx, hardware, d.HardwareRef.Namespace, d.OSImageURL)
	if err != nil {
		return nil, err
	}

	// Create Workflow to match the template and server
	server := tinktypes.Hardware{Hardware: hardware}
	if err = d.WorkflowClient.CreateWorkflow(ctx, userdata, server.Name, client.ProvisionWorkerNodeTemplate, server); err != nil {
		return nil, err
	}

	// Set the HardwareID with machine UID. The hardware object is claimed by the machine.
	if err = d.HardwareClient.SetHardwareID(ctx, hardware, string(meta.UID)); err != nil {
		return nil, err
	}

	return &server, nil
}

func (d *driver) Validate(_ runtime.RawExtension) error {
	return nil
}

func (d *driver) DeprovisionServer(ctx context.Context) error {
	// Get the hardware object from tinkerbell cluster
	targetHardware, err := d.HardwareClient.GetHardware(ctx, d.HardwareRef)
	if err != nil {
		return err
	}

	// Delete the associated Workflow.
	workflowName := targetHardware.Name + "-workflow" // Assuming workflow names are derived from hardware names
	if err := d.WorkflowClient.DeleteWorkflow(ctx, workflowName, targetHardware.Namespace); err != nil {
		return fmt.Errorf("failed to delete workflow %s: %w", workflowName, err)
	}

	// Reset the hardware ID and state in the tinkerbell cluster.
	if err := d.HardwareClient.SetHardwareID(ctx, targetHardware, ""); err != nil {
		return fmt.Errorf("failed to reset hardware ID for %s: %w", targetHardware.Name, err)
	}

	return nil
}

func GetConfig(driverConfig tinktypes.TinkerbellPluginSpec, valueFromStringOrEnvVar func(configVar providerconfigtypes.ConfigVarString, envVarName string) (string, error)) (*tinktypes.Config, error) {
	config := tinktypes.Config{}
	var err error
	// Kubeconfig was specified directly in the Machine/MachineDeployment CR. In this case we need to ensure that the value is base64 encoded.
	if driverConfig.Auth.Kubeconfig.Value != "" {
		val, err := base64.StdEncoding.DecodeString(driverConfig.Auth.Kubeconfig.Value)
		if err != nil {
			// An error here means that this is not a valid base64 string
			// We can be more explicit here with the error for visibility. Webhook will return this error if we hit this scenario.
			return nil, fmt.Errorf("failed to decode base64 encoded kubeconfig. Expected value is a base64 encoded Kubeconfig in JSON or YAML format: %w", err)
		}
		config.Kubeconfig = string(val)
	} else {
		// Environment variable or secret reference was used for providing the value of kubeconfig
		// We have to be lenient in this case and allow unencoded values as well.
		// TODO(mq): Replace this field with a reference to a secret instead of having it inlined.
		config.Kubeconfig, err = valueFromStringOrEnvVar(driverConfig.Auth.Kubeconfig, "TINK_KUBECONFIG")
		if err != nil {
			return nil, fmt.Errorf(`failed to get value of "kubeconfig" field: %w`, err)
		}
	}
	config.ClusterName, err = valueFromStringOrEnvVar(driverConfig.ClusterName, "CLUSTER_NAME")
	if err != nil {
		return nil, fmt.Errorf(`failed to get value of "clusterName" field: %w`, err)
	}

	config.OSImageURL, err = valueFromStringOrEnvVar(driverConfig.OSImageURL, "OS_IMAGE_URL")
	if err != nil {
		return nil, fmt.Errorf(`failed to get value of "OSImageURL" field: %w`, err)
	}

	config.RestConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(config.Kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to decode kubeconfig: %w", err)
	}
	return &config, nil
}
