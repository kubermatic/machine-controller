/*
Copyright 2020 The Machine Controller Authors.

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

package anexia

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"net/http"
	"time"

	anxclient "github.com/anexia-it/go-anxcloud/pkg/client"
	anxaddr "github.com/anexia-it/go-anxcloud/pkg/ipam/address"
	"github.com/anexia-it/go-anxcloud/pkg/vsphere"
	anxvm "github.com/anexia-it/go-anxcloud/pkg/vsphere/provisioning/vm"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

const (
	ProvisionedType = "Provisioned"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func (p *provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData,
	userdata string) (instance instance.Instance, retErr error) {
	status := getProviderStatus(machine)

	// ensure conditions are present on machine
	ensureConditions(&status)

	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("unable to get provider config: %w", err)
	}

	ctx := utils.CreateReconcileContext(utils.ReconcileContext{
		Status:       &status,
		UserData:     userdata,
		Config:       config,
		ProviderData: data,
		Machine:      machine,
	})

	client, err := getClient(config.Token)
	if err != nil {
		return nil, err
	}

	// make sure status is reflected in Machine Object
	defer func() {
		// if error occurs during updating the machine object don't override the original error
		retErr = anxtypes.NewMultiError(retErr, updateMachineStatus(machine, status, data.Update))
	}()

	// check whether machine is already provisioning
	if isAlreadyProvisioning(ctx) && status.ProvisioningID == "" {
		err := waitForVM(ctx, client)
		if err != nil {
			return nil, err
		}
		return p.Get(machine, data)
	}

	// provision machine
	err = provisionVM(ctx, client)
	if err != nil {
		return nil, err
	}
	return p.Get(machine, data)
}

func waitForVM(ctx context.Context, client anxclient.Client) error {
	reconcileContext := utils.GetReconcileContext(ctx)
	api := vsphere.NewAPI(client)
	var identifier string
	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		vms, err := api.Search().ByName(ctx, fmt.Sprintf("%%-%s", reconcileContext.Machine.Name))
		if err != nil {
			return false, nil
		}
		if len(vms) < 1 {
			return false, nil
		}
		if len(vms) > 1 {
			return false, errors.New("too many VMs returned by search")
		}
		identifier = vms[0].Identifier
		return true, nil
	})
	if err != nil {
		return err
	}

	reconcileContext.Status.InstanceID = identifier
	return nil
}

func provisionVM(ctx context.Context, client anxclient.Client) error {
	reconcileContext := utils.GetReconcileContext(ctx)
	vmAPI := vsphere.NewAPI(client)

	ctx, cancel := context.WithTimeout(ctx, anxtypes.CreateRequestTimeout)
	defer cancel()

	status := reconcileContext.Status
	if status.ProvisioningID == "" {
		config := reconcileContext.Config
		reservedIP, err := getIPAddress(ctx, client)
		if err != nil {
			return newError(common.CreateMachineError, "failed to reserve IP: %v", err)
		}
		networkInterfaces := []anxvm.Network{{
			NICType: anxtypes.VmxNet3NIC,
			IPs:     []string{reservedIP},
			VLAN:    config.VlanID,
		}}

		vm := vmAPI.Provisioning().VM().NewDefinition(
			config.LocationID,
			"templates",
			config.TemplateID,
			reconcileContext.Machine.Name,
			config.CPUs,
			config.Memory,
			config.DiskSize,
			networkInterfaces,
		)

		vm.Script = base64.StdEncoding.EncodeToString([]byte(reconcileContext.UserData))

		sshKey, err := ssh.NewKey()
		if err != nil {
			return newError(common.CreateMachineError, "failed to generate ssh key: %v", err)
		}
		vm.SSH = sshKey.PublicKey

		provisionResponse, err := vmAPI.Provisioning().VM().Provision(ctx, vm)
		meta.SetStatusCondition(&status.Conditions, v1.Condition{
			Type:    ProvisionedType,
			Status:  v1.ConditionFalse,
			Reason:  "Provisioning",
			Message: "provisioning request was sent",
		})
		if err != nil {
			return newError(common.CreateMachineError, "instance provisioning failed: %v", err)
		}

		// we successfully sent a VM provisioning request to the API, we consider the IP as 'Bound' now
		status.IPState = anxtypes.IPStateBound

		status.ProvisioningID = provisionResponse.Identifier
	}

	instanceID, err := vmAPI.Provisioning().Progress().AwaitCompletion(ctx, status.ProvisioningID)
	if err != nil {
		// something went wrong remove provisioning ID, so we can start from scratch
		status.ProvisioningID = ""
		return newError(common.CreateMachineError, "instance provisioning failed: %v", err)
	}

	status.InstanceID = instanceID
	meta.SetStatusCondition(&status.Conditions, v1.Condition{
		Type:    ProvisionedType,
		Status:  v1.ConditionTrue,
		Reason:  "Provisioned",
		Message: "Machine has been successfully created",
	})

	return nil
}

func getIPAddress(ctx context.Context, client anxclient.Client) (string, error) {
	reconcileContext := utils.GetReconcileContext(ctx)
	status := reconcileContext.Status

	// only use IP if it is still unbound
	if status.ReservedIP != "" && status.IPState == anxtypes.IPStateUnbound {
		klog.Info("resuing already provisioned ip", "IP", status.ReservedIP)
		return status.ReservedIP, nil
	}

	addrAPI := anxaddr.NewAPI(client)
	config := reconcileContext.Config
	res, err := addrAPI.ReserveRandom(ctx, anxaddr.ReserveRandom{
		LocationID: config.LocationID,
		VlanID:     config.VlanID,
		Count:      1,
	})
	if err != nil {
		return "", newError(common.InvalidConfigurationMachineError, "failed to reserve an ip address: %v", err)
	}
	if len(res.Data) < 1 {
		return "", newError(common.InsufficientResourcesMachineError, "no ip address is available for this machine")
	}

	ip := res.Data[0].Address
	status.ReservedIP = ip
	status.IPState = anxtypes.IPStateUnbound

	return ip, nil
}

func isAlreadyProvisioning(ctx context.Context) bool {
	status := utils.GetReconcileContext(ctx).Status
	condition := meta.FindStatusCondition(status.Conditions, ProvisionedType)
	lastChange := condition.LastTransitionTime.Time
	const reasonInProvisioning = "InProvisioning"
	if condition.Reason == reasonInProvisioning && time.Since(lastChange) > 5*time.Minute {
		meta.SetStatusCondition(&status.Conditions, v1.Condition{
			Type:    ProvisionedType,
			Reason:  "ReInitialising",
			Message: "Could not find ongoing VM provisioning",
			Status:  v1.ConditionFalse,
		})
	}

	return condition.Status == v1.ConditionFalse && condition.Reason == reasonInProvisioning
}

func ensureConditions(status *anxtypes.ProviderStatus) {
	conditions := [...]v1.Condition{
		{Type: ProvisionedType, Message: "", Status: v1.ConditionUnknown, Reason: "Initialising"},
	}
	for _, condition := range conditions {
		if meta.FindStatusCondition(status.Conditions, condition.Type) == nil {
			meta.SetStatusCondition(&status.Conditions, condition)
		}
	}
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*anxtypes.Config, *providerconfigtypes.Config, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}
	pConfig := providerconfigtypes.Config{}
	err := json.Unmarshal(s.Value.Raw, &pConfig)
	if err != nil {
		return nil, nil, err
	}

	if pConfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig := anxtypes.RawConfig{}
	if err = json.Unmarshal(pConfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, err
	}

	c := anxtypes.Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, anxtypes.AnxTokenEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'token': %v", err)
	}

	c.CPUs = rawConfig.CPUs
	c.Memory = rawConfig.Memory
	c.DiskSize = rawConfig.DiskSize

	c.LocationID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.LocationID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'locationID': %v", err)
	}

	c.TemplateID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TemplateID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'templateID': %v", err)
	}

	c.VlanID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'vlanID': %v", err)
	}

	return &c, &pConfig, nil
}

// New returns an Anexia provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// AddDefaults adds omitted optional values to the given MachineSpec
func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its ProviderSpec
func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
	config, _, err := p.getConfig(machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if config.Token == "" {
		return errors.New("token is missing")
	}

	if config.CPUs == 0 {
		return errors.New("cpu count is missing")
	}

	if config.DiskSize == 0 {
		return errors.New("disk size is missing")
	}

	if config.Memory == 0 {
		return errors.New("memory size is missing")
	}

	if config.LocationID == "" {
		return errors.New("location id is missing")
	}

	if config.TemplateID == "" {
		return errors.New("template id is missing")
	}

	if config.VlanID == "" {
		return errors.New("vlan id is missing")
	}

	return nil
}

func (p *provider) Get(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	cli, err := getClient(config.Token)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphereAPI := vsphere.NewAPI(cli)

	status := getProviderStatus(machine)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}
	if status.InstanceID == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.GetRequestTimeout)
	defer cancel()

	info, err := vsphereAPI.Info().Get(ctx, status.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("failed get machine info: %w", err)
	}

	return &anexiaInstance{
		info: &info,
	}, nil
}

func (p *provider) GetCloudConfig(_ v1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	cli, err := getClient(config.Token)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphereAPI := vsphere.NewAPI(cli)

	status := getProviderStatus(machine)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.DeleteRequestTimeout)
	defer cancel()

	err = vsphereAPI.Provisioning().VM().Deprovision(ctx, status.InstanceID, false)
	if err != nil {
		var respErr *anxclient.ResponseError
		// Only error if the error was not "not found"
		if !(errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound) {
			return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
		}
	}

	return true, nil
}

func (p *provider) MigrateUID(_ *v1alpha1.Machine, _ k8stypes.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(_ *v1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(_ v1alpha1.MachineList) error {
	return nil
}

func getClient(token string) (anxclient.Client, error) {

	tokenOpt := anxclient.TokenFromString(token)
	return anxclient.New(tokenOpt)
}

func getProviderStatus(machine *v1alpha1.Machine) anxtypes.ProviderStatus {
	var providerStatus anxtypes.ProviderStatus
	status := machine.Status.ProviderStatus
	if status != nil && status.Raw != nil {
		if err := json.Unmarshal(status.Raw, &providerStatus); err != nil {
			klog.Warningf("Unable to parse status from machine object. status was discarded for machine")
			return anxtypes.ProviderStatus{}
		}
	}
	return providerStatus
}

// newError creates a terminal error matching to the provider interface.
func newError(reason common.MachineStatusError, msg string, args ...interface{}) error {
	return cloudprovidererrors.TerminalError{
		Reason:  reason,
		Message: fmt.Sprintf(msg, args...),
	}
}

// updateMachineStatus tries to update the machine status by any means
// an error will lead to a panic
func updateMachineStatus(machine *v1alpha1.Machine, status anxtypes.ProviderStatus, updater cloudprovidertypes.MachineUpdater) error {
	rawStatus, err := json.Marshal(status)
	if err != nil {
		return err
	}
	err = updater(machine, func(machine *v1alpha1.Machine) {
		machine.Status.ProviderStatus = &runtime.RawExtension{
			Raw: rawStatus,
		}
	})

	if err != nil {
		return err
	}

	return nil
}
