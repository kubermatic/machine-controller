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

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
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
	errMachineSpec        = "Failed to parse MachineSpec: %v"
	errOperatingSystem    = "Invalid or not supported operating system specified %q: %v"
	errConnect            = "Failed to connect: %v"
	errInvalidClientID    = "Client ID is missing"
	errInvalidProjectID   = "Project ID is missing"
	errInvalidEmail       = "Email is missing"
	errInvalidPrivateKey  = "Private key is missing"
	errInvalidZone        = "Zone is missing"
	errInvalidMachineType = "Machine type is missing"
	errInvalidDiskSize    = "Disk size must be a positive number"
	errInvalidDiskType    = "Disk type is missing or has wrong type, allowed are 'pd-standard' and 'pd-ssd'"
	errRetrieveInstance   = "Failed to retrieve instance: %v"
	errInsertInstance     = "Failed to insert instance: %v"
	errDeleteInstance     = "Failed to delete instance: %v"
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

// AddDefaults implements the cloud.Provider interface. It reads the MachineSpec and
// applies defaults for provider specific fields
func (p *Provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	// So far nothing to add.
	return spec, nil
}

// Validate implements the cloud.Provider interface. It validates the given
// machine's specification.
func (p *Provider) Validate(spec v1alpha1.MachineSpec) error {
	// Read configuration.
	cfg, err := newConfig(p.resolver, spec.ProviderSpec)
	if err != nil {
		return newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Check configured values.
	if cfg.clientID == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidClientID)
	}
	if cfg.projectID == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidProjectID)
	}
	if cfg.email == "" {
		return newError(common.InvalidConfigurationMachineError, errInvalidEmail)
	}
	if len(cfg.privateKey) == 0 {
		return newError(common.InvalidConfigurationMachineError, errInvalidPrivateKey)
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

// Get implements the cloud.Provider interface. It gets a node that is associated
// with the given machine.
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
	inst, err := svc.Instances.Get(cfg.projectID, cfg.zone, string(machine.UID)).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			if gerr.Code == http.StatusNotFound {
				return nil, errors.ErrInstanceNotFound
			}
		}
		return nil, newError(common.InvalidConfigurationMachineError, errRetrieveInstance, err)
	}
	return &gcpInstance{inst}, nil
}

// GetCloudConfig implements the cloud.Provider interface. It returns the cloud provider specific
// cloud-config, which gets consumed by the kubelet.
func (p *Provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nyiErr
}

// Create implements the cloud.Provider interface. It creates a cloud instance according
// to the given machine.
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
	inst := &compute.Instance{
		Name:              machine.Spec.Name,
		MachineType:       cfg.machineTypeDescriptor(),
		NetworkInterfaces: networkInterfaces,
		Disks:             disks,
		Tags: &compute.Tags{
			Items: []string{
				string(machine.UID),
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

// Create implements the cloud.Provider interface. It deletes the instance associated with the
// machine and all associated resources.
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
	op, err := svc.Instances.Delete(cfg.projectID, cfg.zone, machine.Spec.Name).Do()
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, errDeleteInstance, err)
	}
	err = svc.waitOperation(cfg.projectID, op, timeoutNormal)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, errDeleteInstance, err)
	}
	return true, nil
}

// MachineMetricsLabels implements the cloud.Provider interface. It returns labels used for the
// Prometheus metrics about created machines.
func (p *Provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	// Read configuration.
	cfg, err := newConfig(p.resolver, machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, errMachineSpec, err)
	}
	// Create labels.
	labels := map[string]string{}

	labels["project"] = c.projectID
	labels["zone"] = c.zone
	labels["type"] = c.machineType
	labels["disksize"] = c.diskSize
	labels["disktype"] = c.diskType

	return labels, nil
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
