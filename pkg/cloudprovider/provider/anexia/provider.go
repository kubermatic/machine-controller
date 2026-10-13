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

	"github.com/anexia/go-anxsdk"
	anxsdkcommon "github.com/anexia/go-anxsdk/v1/common"
	"github.com/anexia/go-anxsdk/v1/vsphere"
	"go.uber.org/zap"
	controllerutil "k8c.io/machine-controller/pkg/controller/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8c.io/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "k8c.io/machine-controller/pkg/cloudprovider/errors"
	"k8c.io/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	"k8c.io/machine-controller/sdk/apis/cluster/common"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
	"k8c.io/machine-controller/sdk/providerconfig"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

const (
	ProvisioningType = "Provisioning"
	ProvisionedType  = "Provisioned"

	invalidCredentialsMessage = "Request was rejected due to invalid credentials"

	// defaultCPUPerformanceType matches go-anxsdk NewDefinition default, used
	// when no cpuPerformanceType is configured.
	defaultCPUPerformanceType = vsphere.CPUPerformanceTypePerformance
)

type provider struct {
	configVarResolver providerconfig.ConfigVarResolver
}

func (p *provider) Create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance instance.Instance, retErr error) {
	status, err := getProviderStatus(log, machine)
	if err != nil {
		return nil, err
	}
	log.Debugw("Machine status", "status", status)

	// ensure conditions are present on machine
	ensureConditions(&status)

	config, providerCfg, err := p.getConfig(ctx, log, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	reconcileCtx := reconcileContext{
		Status:         &status,
		UserData:       userdata,
		Config:         *config,
		ProviderData:   data,
		ProviderConfig: providerCfg,
		Machine:        machine,
	}

	sdkClient := getSDKClient(&machine.Name)

	// make sure status is reflected in Machine Object
	defer func() {
		// if error occurs during updating the machine object don't override the original error
		retErr = errors.Join(retErr, updateMachineStatus(machine, status, data.Update))
	}()

	// provision machine
	err = provisionVM(ctx, reconcileCtx, log, sdkClient)
	if err != nil {
		return nil, wrapAnexiaError(err, "failed waiting for vm provisioning")
	}
	return p.Get(ctx, log, machine, data)
}

func provisionVM(ctx context.Context, reconcileContext reconcileContext, log *zap.SugaredLogger, sdkClient *anxsdk.Client) error {
	ctx, cancel := context.WithTimeout(ctx, anxtypes.CreateRequestTimeout)
	defer cancel()

	status := reconcileContext.Status
	if status.ProvisioningID == "" {
		log.Info("Machine does not contain a provisioningID. Starting to provision")

		config := reconcileContext.Config
		networkInterfaces, err := networkInterfacesForProvisioning(ctx, reconcileContext, log, sdkClient.V1().Ipam().Addresses())
		if err != nil {
			return fmt.Errorf("error generating network config for machine: %w", err)
		}

		request := vsphere.ProvisioningRequest{
			Hostname:           reconcileContext.Machine.Name,
			MemoryMB:           new(config.Memory),
			CPUs:               new(config.CPUs),
			DiskGB:             new(config.DiskSize),
			DiskType:           new(vsphere.DiskType(config.DiskPerformanceType)),
			CPUPerformanceType: new(defaultCPUPerformanceType),
			Network:            networkInterfaces,
		}

		if config.CPUPerformanceType != "" {
			request.CPUPerformanceType = new(vsphere.CPUPerformanceType(config.CPUPerformanceType))
		}

		if config.AvailabilityZone != "" {
			request.AvailabilityZone = new(config.AvailabilityZone)
		}

		for _, disk := range config.Disks {
			request.AdditionalDisks = append(request.AdditionalDisks, vsphere.ProvisioningRequestAdditionalDisk{
				GB:   disk.Size,
				Type: disk.PerformanceType,
			})
		}

		request.Script = new(base64.StdEncoding.EncodeToString([]byte(reconcileContext.UserData)))

		providerCfg := reconcileContext.ProviderConfig
		if providerCfg.Network != nil {
			for index, dnsServer := range providerCfg.Network.DNS.Servers {
				switch index {
				case 0:
					request.DNS1 = new(dnsServer)
				case 1:
					request.DNS2 = new(dnsServer)
				case 2:
					request.DNS3 = new(dnsServer)
				case 3:
					request.DNS4 = new(dnsServer)
				}
			}
		}

		if len(config.SSHPublicKeys) > 0 {
			// use provided SSH public key(s) if specified
			request.SSH = new(strings.Join(config.SSHPublicKeys, "\n"))
		} else {
			// We generate a fresh SSH key but will never actually use it - we just want a valid public key to disable password authentication for our fresh VM.
			sshKey, err := ssh.NewKey()
			if err != nil {
				return newError(common.CreateMachineError, "failed to generate ssh key: %v", err)
			}
			request.SSH = new(sshKey.PublicKey)
		}

		provisionResponse, provisionErr := sdkClient.V1().VSphere().Provisioning().ProvisionTemplate(ctx, config.LocationID, config.TemplateID, request)
		if provisionErr != nil {
			meta.SetStatusCondition(&status.Conditions, metav1.Condition{
				Type:    ProvisionedType,
				Status:  metav1.ConditionFalse,
				Reason:  "ProvisioningError",
				Message: fmt.Sprintf("instance provisioning failed: %v", provisionErr.Error()),
			})
			// errors.Join (not kerrors.NewAggregate) so that errors.As still
			// finds the TerminalError - aggregate implements Is but not As.
			return errors.Join(
				newError(common.CreateMachineError, "instance provisioning failed: %v", provisionErr),
				updateMachineStatus(reconcileContext.Machine, *status, reconcileContext.ProviderData.Update),
			)
		}

		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:    ProvisionedType,
			Status:  metav1.ConditionFalse,
			Reason:  ProvisioningType,
			Message: "Provisioning request was sent",
		})

		// we successfully sent a VM provisioning request to the API, we consider the IP as 'Bound' now
		networkStatusMarkIPsBound(status)

		status.ProvisioningID = provisionResponse.TaskIdentifier
		err = updateMachineStatus(reconcileContext.Machine, *status, reconcileContext.ProviderData.Update)
		if err != nil {
			return err
		}
	}

	// We do not wait for the provisioning task to finish here - Get() polls the
	// task and flips this condition to True once the API reports success.
	log.Infow("Provisioning request accepted, waiting for completion", "provisioningID", status.ProvisioningID)

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    ProvisionedType,
		Status:  metav1.ConditionFalse,
		Reason:  ProvisioningType,
		Message: "Provisioning request accepted, waiting for completion",
	})

	return updateMachineStatus(reconcileContext.Machine, *status, reconcileContext.ProviderData.Update)
}

func ensureConditions(status *anxtypes.ProviderStatus) {
	conditions := [...]metav1.Condition{
		{Type: ProvisionedType, Message: "", Status: metav1.ConditionUnknown, Reason: "Initialising"},
	}
	for _, condition := range conditions {
		if meta.FindStatusCondition(status.Conditions, condition.Type) == nil {
			meta.SetStatusCondition(&status.Conditions, condition)
		}
	}
}

func (p *provider) getConfig(ctx context.Context, log *zap.SugaredLogger, provSpec clusterv1alpha1.ProviderSpec) (*resolvedConfig, *providerconfig.Config, error) {
	pconfig, err := providerconfig.GetConfig(provSpec)
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

	resolvedConfig, err := p.resolveConfig(ctx, log, *rawConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error resolving config: %w", err)
	}

	resolvedConfig.SSHPublicKeys = pconfig.SSHPublicKeys

	return resolvedConfig, pconfig, nil
}

// New returns an Anexia provider.
func New(configVarResolver providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// AddDefaults adds omitted optional values to the given MachineSpec.
func (p *provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its ProviderSpec.
func (p *provider) Validate(ctx context.Context, log *zap.SugaredLogger, machinespec clusterv1alpha1.MachineSpec) error {
	config, _, err := p.getConfig(ctx, log, machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	errs := make([]error, 0)
	if config.CPUs == 0 {
		errs = append(errs, errors.New("cpu count is missing"))
	}

	if config.CPUPerformanceType == "" {
		errs = append(errs, errors.New("cpu performance type is missing"))
	}

	if config.DiskSize == 0 {
		errs = append(errs, errors.New("disk size is missing"))
	}

	if config.DiskPerformanceType == "" {
		errs = append(errs, errors.New("disk performance type is missing"))
	}

	for i, disk := range config.Disks {
		if disk.Size == 0 {
			errs = append(errs, fmt.Errorf("disk size for disk %d is missing", i))
		}
		if disk.PerformanceType == "" {
			errs = append(errs, fmt.Errorf("disk performance type for disk %d is missing", i))
		}
	}

	if config.Memory == 0 {
		errs = append(errs, errors.New("memory size is missing"))
	}

	if config.LocationID == "" {
		errs = append(errs, errors.New("location id is missing"))
	}

	if config.TemplateID == "" {
		errs = append(errs, errors.New("no valid template configured"))
	}

	if len(config.Networks) == 0 {
		errs = append(errs, errors.New("no networks configured"))
	} else {
		atLeastOneAddressSourceConfigured := false
		for _, network := range config.Networks {
			if len(network.Prefixes) > 0 {
				atLeastOneAddressSourceConfigured = true
				break
			}
		}
		if !atLeastOneAddressSourceConfigured {
			errs = append(errs, errors.New("none of the configured networks define an address source, cannot create Machines without any IP"))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (p *provider) Get(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, pd *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	sdkClient := getSDKClient(&machine.Name)

	status, err := getProviderStatus(log, machine)
	if err != nil {
		return nil, err
	}

	if status.InstanceID == "" && status.ProvisioningID == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	if status.DeprovisioningID != "" {
		// info endpoint no longer available for vm -> stop here
		return &anexiaInstance{isDeleting: true}, nil
	}

	// possible out-of-band delete of the worker node in the anexia engine
	// CCM deletes the node but here the InstanceID stays
	if status.InstanceID == "" || status.ProvisioningID != "" {
		provisioning, err := sdkClient.V1().VSphere().Provisioning().GetProvisioningProgress(ctx, status.ProvisioningID)
		if err != nil {
			return nil, wrapAnexiaError(err, "failed to get provisioning progress")
		}

		switch provisioning.Status {
		// First, check whether the request is successful. We have to do this ahead of the error checking,
		// because the errors field does not seem to get cleared if the same provisioning task was successful
		// in the next run.
		//
		// See also: VSD-1473
		case vsphere.ProvisioningStatusSuccess:
			// first tag the machine, then assign
			tagErr := p.tagMachineInEngine(ctx, sdkClient, provisioning, machine, pd.Client)
			if tagErr != nil {
				return nil, tagErr
			}

			status.InstanceID = provisioning.VMIdentifier

			// clear ProvisioningID after successfully provisioned
			// if an out-of-band delete happens the ProvisioningID has to be empty in order to create a new machine inside Create() instead of waiting endlessly
			status.ProvisioningID = ""
			meta.SetStatusCondition(&status.Conditions, metav1.Condition{
				Type:    ProvisionedType,
				Status:  metav1.ConditionTrue,
				Reason:  "Provisioned",
				Message: "Machine has been successfully provisioned",
			})
			if err := updateMachineStatus(machine, status, pd.Update); err != nil {
				return nil, fmt.Errorf("failed updating machine status: %w", err)
			}
		case vsphere.ProvisioningStatusFailed:
			// The provisioning task is done and failed. Clear the
			// ProvisioningID so the Machine is not wedged polling a dead task:
			// the next Get() will report the instance as not found and the
			// controller will provision a new VM.
			status.ProvisioningID = ""
			meta.SetStatusCondition(&status.Conditions, metav1.Condition{
				Type:    ProvisionedType,
				Status:  metav1.ConditionFalse,
				Reason:  "ProvisioningError",
				Message: fmt.Sprintf("vm provisioning had errors: %s", strings.Join(provisioning.Errors, ",")),
			})
			if err := updateMachineStatus(machine, status, pd.Update); err != nil {
				return nil, fmt.Errorf("failed updating machine status: %w", err)
			}
			return nil, fmt.Errorf("vm provisioning had errors: %s", strings.Join(provisioning.Errors, ","))
		case vsphere.ProvisioningStatusCancelled:
			status.ProvisioningID = ""
			meta.SetStatusCondition(&status.Conditions, metav1.Condition{
				Type:    ProvisionedType,
				Status:  metav1.ConditionFalse,
				Reason:  "ProvisioningError",
				Message: fmt.Sprintf("vm provisioning was cancelled: %s", strings.Join(provisioning.Errors, ",")),
			})
			if err := updateMachineStatus(machine, status, pd.Update); err != nil {
				return nil, fmt.Errorf("failed updating machine status: %w", err)
			}
			return nil, fmt.Errorf("vm provisioning was cancelled: %s", strings.Join(provisioning.Errors, ","))
		case vsphere.ProvisioningStatusInProgress:
			return &anexiaInstance{isCreating: true}, nil
		default:
			return nil, fmt.Errorf("unexpected provisioning status %q for task %q", provisioning.Status, status.ProvisioningID)
		}
	}

	instance := anexiaInstance{}
	instance.reservedAddresses = networkReservedAddresses(&status)

	timeoutCtx, cancel := context.WithTimeout(ctx, anxtypes.GetRequestTimeout)
	defer cancel()

	info, err := sdkClient.V1().VSphere().Info().Get(timeoutCtx, status.InstanceID)
	if err != nil {
		return nil, wrapAnexiaError(err, "failed getting machine info")
	}
	instance.info = info

	return &instance, nil
}

func (p *provider) tagMachineInEngine(ctx context.Context, sdkClient *anxsdk.Client, provisioning vsphere.ProvisioningProgress, machine *clusterv1alpha1.Machine, kubeClient client.Client) error {
	const (
		akeNodepoolLabelKey           = "ake-nodepool"
		nodepoolEngineIDAnnotationKey = "ake.k8s.anx.io/nodepool-engine-id"
	)

	err := sdkClient.V1().Core().Resources().AssignTag(ctx, provisioning.VMIdentifier, "k8s")
	if err != nil && !anxsdkcommon.IsErrorWithStatusCode(err, http.StatusUnprocessableEntity) {
		return fmt.Errorf("failed to tag VM in engine: %w", err)
	}
	err = sdkClient.V1().Core().Resources().AssignTag(ctx, provisioning.VMIdentifier, "k8s-worker")
	if err != nil && !anxsdkcommon.IsErrorWithStatusCode(err, http.StatusUnprocessableEntity) {
		return fmt.Errorf("failed to tag VM in engine: %w", err)
	}

	// resolve machine deployment
	refMDName, _, err := controllerutil.GetMachineDeploymentNameAndRevisionForMachine(ctx, machine, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get referenced machine deployment name: %w", err)
	}

	// prefer the annotation on the MachineDeployment
	var md clusterv1alpha1.MachineDeployment
	err = kubeClient.Get(ctx, client.ObjectKey{Name: refMDName, Namespace: machine.Namespace}, &md)
	if err != nil {
		return fmt.Errorf("failed to load referenced machine deployment: %w", err)
	}

	npID, found := md.Annotations[nodepoolEngineIDAnnotationKey]
	if !found {
		// fallback to the deprecated way
		npID, found = machine.Labels[akeNodepoolLabelKey]
	}

	if found {
		// tag the vm with the found
		err = sdkClient.V1().Core().Resources().AssignTag(ctx, provisioning.VMIdentifier, "k8s-nodepool:"+npID)
		if err != nil && !anxsdkcommon.IsErrorWithStatusCode(err, http.StatusUnprocessableEntity) {
			return fmt.Errorf("failed to tag VM in engine: %w", err)
		}
	}

	return nil
}

func (p *provider) Cleanup(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (isDeleted bool, retErr error) {
	if inst, err := p.Get(ctx, log, machine, data); err != nil {
		if cloudprovidererrors.IsNotFound(err) {
			return true, nil
		}

		return false, err
	} else if inst.Status() == instance.StatusCreating {
		log.Error("Failed to cleanup machine: instance is still creating")
		return false, nil
	}

	status, err := getProviderStatus(log, machine)
	if err != nil {
		return false, err
	}

	// make sure status is reflected in Machine Object
	defer func() {
		// if error occurs during updating the machine object don't override the original error
		retErr = errors.Join(retErr, updateMachineStatus(machine, status, data.Update))
	}()

	ensureConditions(&status)

	provisioningClient := getSDKClient(&machine.Name).V1().VSphere().Provisioning()

	deleteCtx, cancel := context.WithTimeout(ctx, anxtypes.DeleteRequestTimeout)
	defer cancel()

	// first check whether there is an provisioning ongoing
	if status.DeprovisioningID == "" {
		// Nothing was ever provisioned, so there is nothing to deprovision.
		if status.InstanceID == "" {
			return true, nil
		}

		response, err := provisioningClient.Deprovision(deleteCtx, status.InstanceID, false)
		if err != nil {
			// The VM is already gone, so we are done.
			if anxsdkcommon.IsNotFoundError(err) {
				return true, nil
			}
			return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
		}
		status.DeprovisioningID = response.Identifier
	}

	return isTaskDone(deleteCtx, provisioningClient, status.DeprovisioningID)
}

func isTaskDone(ctx context.Context, provisioningClient *vsphere.ProvisioningClient, progressIdentifier string) (bool, error) {
	if progressIdentifier == "" {
		return true, nil
	}

	response, err := provisioningClient.GetProvisioningProgress(ctx, progressIdentifier)
	if err != nil {
		return false, err
	}

	switch response.Status {
	case vsphere.ProvisioningStatusSuccess:
		return true, nil
	case vsphere.ProvisioningStatusInProgress:
		return false, nil
	case vsphere.ProvisioningStatusCancelled,
		vsphere.ProvisioningStatusFailed:
		taskErrors, _ := json.Marshal(response.Errors)
		return true, fmt.Errorf("task failed with: %s", taskErrors)
	default:
		return false, fmt.Errorf("unexpected provisioning status %q for task %q", response.Status, progressIdentifier)
	}
}

func (p *provider) MigrateUID(_ context.Context, _ *zap.SugaredLogger, _ *clusterv1alpha1.Machine, _ k8stypes.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(_ *clusterv1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}

// getProviderStatus decodes the Anexia ProviderStatus from the Machine object.
//
// A decode failure must not be swallowed: this status is the only place where
// the instance and reserved IPs of a Machine are recorded (nothing is tagged
// with the Machine UID on the Anexia side and MigrateUID is a no-op), so
// treating an unreadable status as "empty" would make the controller provision
// a second VM and orphan the existing one together with its reserved IPs.
func getProviderStatus(log *zap.SugaredLogger, machine *clusterv1alpha1.Machine) (anxtypes.ProviderStatus, error) {
	var providerStatus anxtypes.ProviderStatus
	status := machine.Status.ProviderStatus
	if status != nil && status.Raw != nil {
		if err := json.Unmarshal(status.Raw, &providerStatus); err != nil {
			log.Errorw("Failed to parse status from machine object", "error", err)
			return anxtypes.ProviderStatus{}, fmt.Errorf("failed to parse provider status from machine object: %w", err)
		}
	}
	return providerStatus, nil
}

// newError creates a terminal error matching to the provider interface.
func newError(reason common.MachineStatusError, msg string, args ...any) error {
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

func wrapAnexiaError(err error, msg string) error {
	var sdkError *anxsdkcommon.APIError
	if errors.As(err, &sdkError) {
		if sdkError.StatusCode == http.StatusForbidden || sdkError.StatusCode == http.StatusUnauthorized {
			return cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: invalidCredentialsMessage,
			}
		}

		if sdkError.StatusCode == http.StatusNotFound {
			return cloudprovidererrors.ErrInstanceNotFound
		}
	}

	return fmt.Errorf("%s: %w", msg, err)
}
