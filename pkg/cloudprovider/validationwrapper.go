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

package cloudprovider

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"

	"k8s.io/apimachinery/pkg/types"
)

type cachingValidationWrapper struct {
	actualProvider cloudprovidertypes.Provider
}

// NewValidationCacheWrappingCloudProvider returns a wrapped cloudprovider.
func NewValidationCacheWrappingCloudProvider(actualProvider cloudprovidertypes.Provider) cloudprovidertypes.Provider {
	return &cachingValidationWrapper{actualProvider: actualProvider}
}

// AddDefaults just calls the underlying cloudproviders AddDefaults.
func (w *cachingValidationWrapper) AddDefaults(log *zap.SugaredLogger, spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return w.actualProvider.AddDefaults(log, spec)
}

// Validate tries to get the validation result from the cache and if not found, calls the
// cloudproviders Validate and saves that to the cache.
func (w *cachingValidationWrapper) Validate(ctx context.Context, log *zap.SugaredLogger, spec v1alpha1.MachineSpec) error {
	result, exists, err := cache.Get(spec)
	if err != nil {
		return fmt.Errorf("error getting validation result from cache: %w", err)
	}
	if exists {
		log.Debug("Got cache hit for validation")
		return result
	}

	log.Debug("Got cache miss for validation")
	err = w.actualProvider.Validate(ctx, log, spec)

	// do not cache canceled contexts (e.g. the validation request was canceled client-side)
	// and timeouts (assumed to be temporary)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if err := cache.Set(spec, err); err != nil {
			return fmt.Errorf("failed to set cache after validation: %w", err)
		}
	}

	return err
}

// Get just calls the underlying cloudproviders Get.
func (w *cachingValidationWrapper) Get(ctx context.Context, log *zap.SugaredLogger, machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	return w.actualProvider.Get(ctx, log, machine, data)
}

// GetCloudConfig just calls the underlying cloudproviders GetCloudConfig.
func (w *cachingValidationWrapper) GetCloudConfig(spec v1alpha1.MachineSpec) (string, string, error) {
	return w.actualProvider.GetCloudConfig(spec)
}

// Create just calls the underlying cloudproviders Create.
func (w *cachingValidationWrapper) Create(ctx context.Context, log *zap.SugaredLogger, machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	return w.actualProvider.Create(ctx, log, machine, data, userdata)
}

// Cleanup just calls the underlying cloudproviders Cleanup.
func (w *cachingValidationWrapper) Cleanup(ctx context.Context, log *zap.SugaredLogger, m *v1alpha1.Machine, mcd *cloudprovidertypes.ProviderData) (bool, error) {
	return w.actualProvider.Cleanup(ctx, log, m, mcd)
}

// MigrateUID just calls the underlying cloudproviders MigrateUID.
func (w *cachingValidationWrapper) MigrateUID(ctx context.Context, log *zap.SugaredLogger, m *v1alpha1.Machine, newUID types.UID) error {
	return w.actualProvider.MigrateUID(ctx, log, m, newUID)
}

// MachineMetricsLabels just calls the underlying cloudproviders MachineMetricsLabels.
func (w *cachingValidationWrapper) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	return w.actualProvider.MachineMetricsLabels(machine)
}

func (w *cachingValidationWrapper) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return w.actualProvider.SetMetricsForMachines(machines)
}
