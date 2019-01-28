package cloudprovider

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type cachingValidationWrapper struct {
	actualProvider cloud.Provider
}

// NewValidationCacheWrappingCloudProvider returns a wrapped cloudprovider
func NewValidationCacheWrappingCloudProvider(actualProvider cloud.Provider) cloud.Provider {
	return &cachingValidationWrapper{actualProvider: actualProvider}
}

// AddDefaults just calls the underlying cloudproviders AddDefaults
func (w *cachingValidationWrapper) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return w.actualProvider.AddDefaults(spec)
}

// Validate tries to get the validation result from the cache and if not found, calls the
// cloudproviders Validate and saves that to the cache
func (w *cachingValidationWrapper) Validate(spec v1alpha1.MachineSpec) error {
	result, exists, err := cache.Get(spec)
	if err != nil {
		return fmt.Errorf("error getting validation result from cache: %v", err)
	}
	if exists {
		glog.V(6).Infof("Got cache hit for validation")
		return result
	}

	glog.V(6).Infof("Got cache miss for validation")
	err = w.actualProvider.Validate(spec)
	if err := cache.Set(spec, err); err != nil {
		return fmt.Errorf("failed to set cache after validation: %v", err)
	}

	return err
}

// Get just calls the underlying cloudproviders Get
func (w *cachingValidationWrapper) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	return w.actualProvider.Get(machine)
}

// GetCloudConfig just calls the underlying cloudproviders GetCloudConfig
func (w *cachingValidationWrapper) GetCloudConfig(spec v1alpha1.MachineSpec) (string, string, error) {
	return w.actualProvider.GetCloudConfig(spec)
}

// Create just calls the underlying cloudproviders Create
func (w *cachingValidationWrapper) Create(m *v1alpha1.Machine, mcd *cloud.MachineCreateDeleteData, cloudConfig string) (instance.Instance, error) {
	return w.actualProvider.Create(m, mcd, cloudConfig)
}

// Cleanup just calls the underlying cloudproviders Cleanup
func (w *cachingValidationWrapper) Cleanup(m *v1alpha1.Machine, mcd *cloud.MachineCreateDeleteData) (bool, error) {
	return w.actualProvider.Cleanup(m, mcd)
}

// MigrateUID just calls the underlying cloudproviders MigrateUID
func (w *cachingValidationWrapper) MigrateUID(m *v1alpha1.Machine, new types.UID) error {
	return w.actualProvider.MigrateUID(m, new)
}

// MachineMetricsLabels just calls the underlying cloudproviders MachineMetricsLabels
func (w *cachingValidationWrapper) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	return w.actualProvider.MachineMetricsLabels(machine)
}

func (w *cachingValidationWrapper) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return w.actualProvider.SetMetricsForMachines(machines)
}
