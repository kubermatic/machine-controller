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
	"net/http"
	"strings"
	"sync"
	"time"

	"go.anx.io/go-anxcloud/pkg/api"
	corev1 "go.anx.io/go-anxcloud/pkg/apis/core/v1"
	vspherev1 "go.anx.io/go-anxcloud/pkg/apis/vsphere/v1"
	"go.anx.io/go-anxcloud/pkg/client"
	anxclient "go.anx.io/go-anxcloud/pkg/client"
	anxaddr "go.anx.io/go-anxcloud/pkg/ipam/address"
	"go.anx.io/go-anxcloud/pkg/vsphere"
	"go.anx.io/go-anxcloud/pkg/vsphere/provisioning/progress"
	anxvm "go.anx.io/go-anxcloud/pkg/vsphere/provisioning/vm"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

const (
	ProvisionedType = "Provisioned"
)

var (
	// ErrConfigDiskSizeAndDisks is returned when the config has both DiskSize and Disks set, which is unsupported.
	ErrConfigDiskSizeAndDisks = errors.New("both the deprecated DiskSize and new Disks attribute are set")

	// ErrMultipleDisksNotYetImplemented is returned when multiple disks are configured.
	ErrMultipleDisksNotYetImplemented = errors.New("multiple disks configured, but this feature is not yet implemented")
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// resolvedDisk contains the resolved values from types.RawDisk.
type resolvedDisk struct {
	anxtypes.RawDisk

	PerformanceType string
}

// resolvedConfig contains the resolved values from types.RawConfig.
type resolvedConfig struct {
	anxtypes.RawConfig

	Token      string
	VlanID     string
	LocationID string
	TemplateID string

	Disks []resolvedDisk
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance instance.Instance, retErr error) {
	status := getProviderStatus(machine)
	klog.V(3).Infof(fmt.Sprintf("'%s' has status %#v", machine.Name, status))

	// ensure conditions are present on machine
	ensureConditions(&status)

	config, _, err := p.getConfig(ctx, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("unable to get provider config: %w", err)
	}

	ctx = createReconcileContext(ctx, reconcileContext{
		Status:       &status,
		UserData:     userdata,
		Config:       *config,
		ProviderData: data,
		Machine:      machine,
	})

	_, client, err := getClient(config.Token)
	if err != nil {
		return nil, err
	}

	// make sure status is reflected in Machine Object
	defer func() {
		// if error occurs during updating the machine object don't override the original error
		retErr = anxtypes.NewMultiError(retErr, updateMachineStatus(machine, status, data.Update))
	}()

	// provision machine
	err = provisionVM(ctx, client)
	if err != nil {
		return nil, anexiaErrorToTerminalError(err, "failed waiting for vm provisioning")
	}
	return p.Get(ctx, machine, data)
}

func provisionVM(ctx context.Context, client anxclient.Client) error {
	reconcileContext := getReconcileContext(ctx)
	vmAPI := vsphere.NewAPI(client)

	ctx, cancel := context.WithTimeout(ctx, anxtypes.CreateRequestTimeout)
	defer cancel()

	status := reconcileContext.Status
	if status.ProvisioningID == "" {
		klog.V(2).Info(fmt.Sprintf("Machine '%s'  does not contain a provisioningID yet. Starting to provision",
			reconcileContext.Machine.Name))

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
			config.Disks[0].Size,
			networkInterfaces,
		)

		vm.DiskType = config.Disks[0].PerformanceType

		vm.Script = base64.StdEncoding.EncodeToString([]byte(reconcileContext.UserData))

		// We generate a fresh SSH key but will never actually use it - we just want a valid public key to disable password authentication for our fresh VM.
		sshKey, err := ssh.NewKey()
		if err != nil {
			return newError(common.CreateMachineError, "failed to generate ssh key: %v", err)
		}
		vm.SSH = sshKey.PublicKey

		provisionResponse, err := vmAPI.Provisioning().VM().Provision(ctx, vm, false)
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
		err = updateMachineStatus(reconcileContext.Machine, *status, reconcileContext.ProviderData.Update)
		if err != nil {
			return err
		}
	}

	klog.V(2).Info(fmt.Sprintf("Using provisionID from machine '%s' to await completion",
		reconcileContext.Machine.Name))

	meta.SetStatusCondition(&status.Conditions, v1.Condition{
		Type:    ProvisionedType,
		Status:  v1.ConditionTrue,
		Reason:  "Provisioned",
		Message: "Machine has been successfully created",
	})

	return updateMachineStatus(reconcileContext.Machine, *status, reconcileContext.ProviderData.Update)
}

var _engsup3404mutex sync.Mutex

func getIPAddress(ctx context.Context, client anxclient.Client) (string, error) {
	reconcileContext := getReconcileContext(ctx)
	status := reconcileContext.Status

	// only use IP if it is still unbound
	if status.ReservedIP != "" && status.IPState == anxtypes.IPStateUnbound {
		klog.Infof("reusing already provisioned ip %q", status.ReservedIP)
		return status.ReservedIP, nil
	}

	_engsup3404mutex.Lock()
	defer _engsup3404mutex.Unlock()

	klog.Info(fmt.Sprintf("Creating a new IP for machine %q", reconcileContext.Machine.Name))
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
	status := getReconcileContext(ctx).Status
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

func resolveTemplateID(ctx context.Context, a api.API, config anxtypes.RawConfig, configVarResolver *providerconfig.ConfigVarResolver, locationID string) (string, error) {
	templateName, err := configVarResolver.GetConfigVarStringValue(config.Template)
	if err != nil {
		return "", fmt.Errorf("failed to get 'template': %w", err)
	}

	templateBuild, err := configVarResolver.GetConfigVarStringValue(config.TemplateBuild)
	if err != nil {
		return "", fmt.Errorf("failed to get 'templateBuild': %w", err)
	}

	template, err := vspherev1.FindNamedTemplate(ctx, a, templateName, templateBuild, corev1.Location{Identifier: locationID})
	if err != nil {
		return "", fmt.Errorf("failed to retrieve named template: %w", err)
	}

	return template.Identifier, nil
}

func (p *provider) resolveConfig(ctx context.Context, config anxtypes.RawConfig) (*resolvedConfig, error) {
	var err error
	ret := resolvedConfig{
		RawConfig: config,
	}

	ret.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(config.Token, anxtypes.AnxTokenEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'token': %w", err)
	}

	ret.LocationID, err = p.configVarResolver.GetConfigVarStringValue(config.LocationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'locationID': %w", err)
	}

	ret.TemplateID, err = p.configVarResolver.GetConfigVarStringValue(config.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'templateID': %w", err)
	}

	// when "templateID" is not set, we expect "template" to be
	if ret.TemplateID == "" {
		a, _, err := getClient(ret.Token)
		if err != nil {
			return nil, fmt.Errorf("failed initializing API clients: %w", err)
		}

		templateID, err := resolveTemplateID(ctx, a, config, p.configVarResolver, ret.LocationID)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving template id from named template: %w", err)
		}

		ret.TemplateID = templateID
	}

	ret.VlanID, err = p.configVarResolver.GetConfigVarStringValue(config.VlanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'vlanID': %w", err)
	}

	if config.DiskSize != 0 {
		if len(config.Disks) != 0 {
			return nil, ErrConfigDiskSizeAndDisks
		}

		klog.Warningf("Configuration uses the deprecated DiskSize attribute, please migrate to the Disks array instead.")

		config.Disks = []anxtypes.RawDisk{
			{
				Size: config.DiskSize,
			},
		}
		config.DiskSize = 0
	}

	ret.Disks = make([]resolvedDisk, len(config.Disks))

	for idx, disk := range config.Disks {
		ret.Disks[idx].RawDisk = disk

		ret.Disks[idx].PerformanceType, err = p.configVarResolver.GetConfigVarStringValue(disk.PerformanceType)
		if err != nil {
			return nil, fmt.Errorf("failed to get 'performanceType' of disk %v: %w", idx, err)
		}
	}

	return &ret, nil
}

func (p *provider) getConfig(ctx context.Context, provSpec clusterv1alpha1.ProviderSpec) (*resolvedConfig, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}
	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := anxtypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing provider config: %w", err)
	}

	resolvedConfig, err := p.resolveConfig(ctx, *rawConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error resolving config: %w", err)
	}

	return resolvedConfig, pconfig, nil
}

// New returns an Anexia provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// AddDefaults adds omitted optional values to the given MachineSpec.
func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its ProviderSpec.
func (p *provider) Validate(ctx context.Context, machinespec clusterv1alpha1.MachineSpec) error {
	config, _, err := p.getConfig(ctx, machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if config.Token == "" {
		return errors.New("token not set")
	}

	if config.CPUs == 0 {
		return errors.New("cpu count is missing")
	}

	if len(config.Disks) == 0 {
		return errors.New("no disks configured")
	}

	if len(config.Disks) > 1 {
		return ErrMultipleDisksNotYetImplemented
	}

	for _, disk := range config.Disks {
		if disk.Size == 0 {
			return errors.New("disk size is missing")
		}
	}

	if config.Memory == 0 {
		return errors.New("memory size is missing")
	}

	if config.LocationID == "" {
		return errors.New("location id is missing")
	}

	if config.TemplateID == "" {
		return errors.New("no valid template configured")
	}

	if config.VlanID == "" {
		return errors.New("vlan id is missing")
	}

	return nil
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, pd *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, err := p.getConfig(ctx, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to retrieve config: %v", err)
	}

	_, cli, err := getClient(config.Token)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphereAPI := vsphere.NewAPI(cli)

	status := getProviderStatus(machine)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	if status.InstanceID == "" && status.ProvisioningID == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	if status.InstanceID == "" {
		progress, err := vsphereAPI.Provisioning().Progress().Get(ctx, status.ProvisioningID)
		if err != nil {
			return nil, anexiaErrorToTerminalError(err, "failed to get provisioning progress")
		}
		if len(progress.Errors) > 0 {
			return nil, fmt.Errorf("vm provisioning had errors: %s", strings.Join(progress.Errors, ","))
		}
		if progress.Progress < 100 || progress.VMIdentifier == "" {
			return &anexiaInstance{isCreating: true}, nil
		}

		status.InstanceID = progress.VMIdentifier

		if err := updateMachineStatus(machine, status, pd.Update); err != nil {
			return nil, fmt.Errorf("failed updating machine status: %w", err)
		}
	}

	instance := anexiaInstance{}

	if status.IPState == anxtypes.IPStateBound && status.ReservedIP != "" {
		instance.reservedAddresses = []string{status.ReservedIP}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, anxtypes.GetRequestTimeout)
	defer cancel()

	info, err := vsphereAPI.Info().Get(timeoutCtx, status.InstanceID)
	if err != nil {
		return nil, anexiaErrorToTerminalError(err, "failed getting machine info")
	}
	instance.info = &info

	return &instance, nil
}

func (p *provider) GetCloudConfig(_ clusterv1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (isDeleted bool, retErr error) {
	status := getProviderStatus(machine)
	// make sure status is reflected in Machine Object
	defer func() {
		// if error occurs during updating the machine object don't override the original error
		retErr = anxtypes.NewMultiError(retErr, updateMachineStatus(machine, status, data.Update))
	}()

	ensureConditions(&status)
	config, _, err := p.getConfig(ctx, machine.Spec.ProviderSpec)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	_, cli, err := getClient(config.Token)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphereAPI := vsphere.NewAPI(cli)

	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	deleteCtx, cancel := context.WithTimeout(ctx, anxtypes.DeleteRequestTimeout)
	defer cancel()

	// first check whether there is an provisioning ongoing
	if status.DeprovisioningID == "" {
		response, err := vsphereAPI.Provisioning().VM().Deprovision(deleteCtx, status.InstanceID, false)
		if err != nil {
			var respErr *anxclient.ResponseError
			// Only error if the error was not "not found"
			if !(errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound) {
				return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
			}
		}
		status.DeprovisioningID = response.Identifier
	}

	return isTaskDone(deleteCtx, cli, status.DeprovisioningID)
}

func isTaskDone(ctx context.Context, cli anxclient.Client, progressIdentifier string) (bool, error) {
	response, err := progress.NewAPI(cli).Get(ctx, progressIdentifier)
	if err != nil {
		return false, err
	}

	if len(response.Errors) != 0 {
		taskErrors, _ := json.Marshal(response.Errors)
		return true, fmt.Errorf("task failed with: %s", taskErrors)
	}

	if response.Progress == 100 {
		return true, nil
	}

	return false, nil
}

func (p *provider) MigrateUID(_ context.Context, _ *clusterv1alpha1.Machine, _ k8stypes.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(_ *clusterv1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}

func getClient(token string) (api.API, anxclient.Client, error) {
	tokenOpt := anxclient.TokenFromString(token)
	client := anxclient.HTTPClient(&http.Client{Timeout: 120 * time.Second})

	a, err := api.NewAPI(api.WithClientOptions(client, tokenOpt))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating generic API client: %w", err)
	}

	legacyClient, err := anxclient.New(tokenOpt, client)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating legacy client: %w", err)
	}

	return a, legacyClient, nil
}

func getProviderStatus(machine *clusterv1alpha1.Machine) anxtypes.ProviderStatus {
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
// an error will lead to a panic.
func updateMachineStatus(machine *clusterv1alpha1.Machine, status anxtypes.ProviderStatus, updater cloudprovidertypes.MachineUpdater) error {
	rawStatus, err := json.Marshal(status)
	if err != nil {
		return err
	}
	err = updater(machine, func(machine *clusterv1alpha1.Machine) {
		machine.Status.ProviderStatus = &runtime.RawExtension{
			Raw: rawStatus,
		}
	})

	if err != nil {
		return err
	}

	return nil
}

func anexiaErrorToTerminalError(err error, msg string) error {
	var httpError api.HTTPError
	if errors.As(err, &httpError) && (httpError.StatusCode() == http.StatusForbidden || httpError.StatusCode() == http.StatusUnauthorized) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "Request was rejected due to invalid credentials",
		}
	}

	var responseError *client.ResponseError
	if errors.As(err, &responseError) && (responseError.ErrorData.Code == http.StatusForbidden || responseError.ErrorData.Code == http.StatusUnauthorized) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "Request was rejected due to invalid credentials",
		}
	}

	return fmt.Errorf("%s: %w", msg, err)
}
