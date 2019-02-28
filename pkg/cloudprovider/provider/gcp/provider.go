//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

//-----
// Imports
//-----

import (
	"fmt"
	"net/http"
	"strconv"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

//-----
// Constants
//-----

// Terminal error messages.
const (
	errMachineSpec           = "Failed to parse MachineSpec: %v"
	errOperatingSystem       = "Invalid or not supported operating system specified %q: %v"
	errConnect               = "Failed to connect: %v"
	errInvalidServiceAccount = "Service account is missing"
	errInvalidZone           = "Zone is missing"
	errInvalidMachineType    = "Machine type is missing"
	errInvalidDiskSize       = "Disk size must be a positive number"
	errInvalidDiskType       = "Disk type is missing or has wrong type, allowed are 'pd-standard' and 'pd-ssd'"
	errRetrieveInstance      = "Failed to retrieve instance: %v"
	errGotTooManyInstances   = "Got more than 1 instance matching the machine UID label"
	errCloudConfig           = "Failed to convert cloud-config to string: %v"
	errInsertInstance        = "Failed to insert instance: %v"
	errDeleteInstance        = "Failed to delete instance: %v"
	errSetLabels             = "Failed to set the labels for the new machine UID: %v"
)

// Instance labels.
const (
	labelMachineName = "machine_name"
	labelMachineUID  = "machine_uid"
)

// nyiErr is a temporary error used during implementation. Has to be removed.
var nyiErr = fmt.Errorf("not yet implemented")

//-----
// Provider
//-----

// Compile time verification of Provider implementing cloud.Provider.
var _ cloud.Provider = New(nil)

// Provider implements the cloud.Provider interface for the Google Cloud Platform.
type Provider struct {
	resolver *providerconfig.ConfigVarResolver
}

// New creates a cloud provider instance for the Google Cloud Platform.
func New(configVarResolver *providerconfig.ConfigVarResolver) *Provider {
	return &Provider{
		resolver: configVarResolver,
	}
}

// AddDefaults reads the MachineSpec and applies defaults for provider specific fields
func (p *Provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	// TODO(mue) Check for defaults to apply.
	return spec, nil
}

// Validate checks the given machine's specification.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	// Read configuration.
	cfg, err := newConfig(p.resolver, spec.ProviderSpec)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Check configured values.
	if cfg.serviceAccount == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidServiceAccount)
	}
	if cfg.zone == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidZone)
	}
	if cfg.machineType == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidMachineType)
	}
	if cfg.diskSize < 1 {
		return newError(common.InvalidConfigurationMachineError, errInvalidDiskSize)
	}
	if !diskTypes[cfg.diskType] {
		return newError(common.InvalidConfigurationMachineError, errInvalidDiskType)
	}
	_, err = cfg.sourceImageDescriptor()
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errOperatingSystem, cfg.providerConfig.OperatingSystem, err)
	}
	return nil
}

// Get retrieves a node instance that is associated with the given machine.
func (p *Provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Retrieve instance.
	label := fmt.Sprintf("labels.%s=%s", labelMachineUID, machine.UID)
	insts, err := svc.Instances.List(cfg.projectID, cfg.zone).Filter(label).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			if gerr.Code == http.StatusNotFound {
				return nil, errors.ErrInstanceNotFound
			}
		}
		return nil, newError(common.InvalidConfigurationMachineError, errRetrieveInstance, err)
	}
	if len(insts.Items) == 0 {
		return nil, errors.ErrInstanceNotFound
	}
	if len(insts.Items) > 1 {
		return nil, newError(common.InvalidConfigurationMachineError, errGotTooManyInstances)
	}
	return &gcpInstance{insts.Items[0]}, nil

}

// GetCloudConfig returns the cloud provider specific cloud-config for the kubelet.
func (p *Provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, spec.ProviderSpec)
	if err != nil {
		return "", "", newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Init cloud configuration.
	// TODO(mue) Check for needed fields and extend configuration.
	cc := &cloudConfig{
		Global: global{
			ProjectID: cfg.projectID,
			LocalZone: cfg.zone,
		},
	}
	config, err = cc.asString()
	if err != nil {
		return "", "", newError(common.InvalidConfigurationMachineError, errCloudConfig, err)
	}
	return config, "gce", nil
}

// Create inserts a cloud instance according to the given machine.
func (p *Provider) Create(
	machine *v1alpha1.Machine,
	data *cloud.MachineCreateDeleteData,
	userdata string,
) (instance.Instance, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Create GCP instance spec and insert it.
	networkInterfaces, err := svc.networkInterfaces(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	disks, err := svc.attachedDisks(cfg)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	labels := map[string]string{}
	for k, v := range cfg.labels {
		labels[k] = v
	}
	labels[labelMachineName] = machine.Spec.Name
	labels[labelMachineUID] = string(machine.UID)
	inst := &compute.Instance{
		Name:              machine.Spec.Name,
		MachineType:       cfg.machineTypeDescriptor(),
		NetworkInterfaces: networkInterfaces,
		Disks:             disks,
		Labels:            labels,
		ServiceAccounts: []*compute.ServiceAccounts{
			{
				Email: cfg.jwtConfig.Email,
				Scopes: []string{
					compute.ComputeScope,
					compute.DevstorageReadOnlyScope,
					"https://www.googleapis.com/auth/monitoring",
					"https://www.googleapis.com/auth/logging.write",
				},
			},
		},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				{
					Key:   "user-data",
					Value: &userdata,
				},
			},
		},
	}
	op, err := svc.Instances.Insert(cfg.projectID, cfg.zone, inst).Do()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errInsertInstance, err)
	}
	err = svc.waitOperation(cfg.projectID, op, timeoutNormal)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errInsertInstance, err)
	}
	// Retrieve it to get a full qualified instance.
	return p.Get(machine)
}

// Cleanup deletes the instance associated with the machine and all associated resources.
func (p *Provider) Cleanup(
	machine *v1alpha1.Machine,
	data *cloud.MachineCreateDeleteData,
) (bool, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Delete instance.
	op, err := svc.Instances.Delete(cfg.projectID, cfg.zone, machine.Spec.Name).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			if gerr.Code == http.StatusNotFound {
				return true, nil
			}
		}
		return false, newError(common.InvalidConfigurationMachineError, errDeleteInstance, err)
	}
	err = svc.waitOperation(cfg.projectID, op, timeoutNormal)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, errDeleteInstance, err)
	}
	return false, nil
}

// MachineMetricsLabels returns labels used for the  Prometheus metrics about created machines.
func (p *Provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Create labels.
	labels := map[string]string{}

	labels["project"] = cfg.projectID
	labels["zone"] = cfg.zone
	labels["type"] = cfg.machineType
	labels["disksize"] = strconv.FormatInt(cfg.diskSize, 10)
	labels["disktype"] = cfg.diskType

	return labels, nil
}

// MigrateUID updates the UID of an instance after the controller migrates types
// and the UID of the machine object changed.
func (p *Provider) MigrateUID(machine *v1alpha1.Machine, newUID types.UID) error {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Connect to GCP.
	svc, err := connectComputeService(cfg)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errConnect, err)
	}
	// Create new labels and set them.
	labels := map[string]string{}
	for k, v := range cfg.labels {
		labels[k] = v
	}
	labels[labelMachineName] = machine.Spec.Name
	labels[labelMachineUID] = string(newUID)
	slReq := &compute.InstancesSetLabelsRequest{
		Labels: labels,
	}
	op, err := svc.Instances.SetLabels(cfg.projectID, cfg.zone, machine.Spec.Name, slReq).Do()
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errSetLabels, err)
	}
	err = svc.waitOperation(cfg.projectID, op, timeoutNormal)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errSetLabels, err)
	}
	return nil
}

// SetMetricsForMachines allows providers to provide provider-specific metrics.
func (p *Provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	// TODO(mue) Check what to return for GCP.
	return nil
}

//-----
// Private helpers
//-----

// newError creates a terminal error matching to the provider interface.
func newError(reason common.MachineStatusError, msg string, args ...interface{}) error {
	return errors.TerminalError{
		Reason:  reason,
		Message: fmt.Sprintf(msg, args...),
	}
}
