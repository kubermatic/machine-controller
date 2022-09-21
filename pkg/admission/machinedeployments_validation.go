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

package admission

import (
	"encoding/json"
	"fmt"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	osmresources "k8c.io/operating-system-manager/pkg/controllers/osc/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const ospNamePattern = "osp-%s"

func validateMachineDeployment(md v1alpha1.MachineDeployment) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateMachineDeploymentSpec(&md.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateMachineDeploymentSpec(spec *v1alpha1.MachineDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, metav1validation.ValidateLabelSelector(&spec.Selector, fldPath.Child("selector"))...)
	if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is not valid for MachineDeployment."))
	}
	selector, err := metav1.LabelSelectorAsSelector(&spec.Selector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "invalid label selector."))
	} else {
		labels := labels.Set(spec.Template.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("template", "metadata", "labels"), spec.Template.Labels, "`selector` does not match template `labels`"))
		}
	}
	if spec.Replicas == nil || *spec.Replicas < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("replicas"), *spec.Replicas, "replicas must be specified and can not be negative"))
	}
	allErrs = append(allErrs, validateMachineDeploymentStrategy(spec.Strategy, fldPath.Child("strategy"))...)
	return allErrs
}

func validateMachineDeploymentStrategy(strategy *v1alpha1.MachineDeploymentStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch strategy.Type {
	case common.RollingUpdateMachineDeploymentStrategyType:
		if strategy.RollingUpdate != nil {
			allErrs = append(allErrs, validateMachineRollingUpdateDeployment(strategy.RollingUpdate, fldPath.Child("rollingUpdate"))...)
		}
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("Type"), strategy.Type, "is an invalid type"))
	}
	return allErrs
}

func validateMachineRollingUpdateDeployment(rollingUpdate *v1alpha1.MachineRollingUpdateDeployment, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	var maxUnavailable int
	var maxSurge int
	if rollingUpdate.MaxUnavailable != nil {
		allErrs = append(allErrs, validatePositiveIntOrPercent(rollingUpdate.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
		maxUnavailable, _ = getIntOrPercent(rollingUpdate.MaxUnavailable, false)
		// Validate that MaxUnavailable is not more than 100%.
		if len(utilvalidation.IsValidPercent(rollingUpdate.MaxUnavailable.StrVal)) == 0 && maxUnavailable > 100 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), rollingUpdate.MaxUnavailable, "should not be more than 100%"))
		}
	}
	if rollingUpdate.MaxSurge != nil {
		allErrs = append(allErrs, validatePositiveIntOrPercent(rollingUpdate.MaxSurge, fldPath.Child("maxSurge"))...)
		maxSurge, _ = getIntOrPercent(rollingUpdate.MaxSurge, true)
	}
	if rollingUpdate.MaxUnavailable != nil && rollingUpdate.MaxSurge != nil && maxUnavailable == 0 && maxSurge == 0 {
		// Both MaxSurge and MaxUnavailable cannot be zero.
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), rollingUpdate.MaxUnavailable, "may not be 0 when `maxSurge` is 0"))
	}
	return allErrs
}

func validatePositiveIntOrPercent(s *intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if x, err := getIntOrPercent(s, false); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, s.StrVal, "value should be int(5) or percentage(5%)"))
	} else if x < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, x, "value should not be negative"))
	}
	return allErrs
}

func getIntOrPercent(s *intstr.IntOrString, roundUp bool) (int, error) {
	return intstr.GetValueFromIntOrPercent(s, 100, roundUp)
}

func machineDeploymentDefaultingFunction(md *v1alpha1.MachineDeployment) {
	v1alpha1.PopulateDefaultsMachineDeployment(md)
}

func mutationsForMachineDeployment(md *v1alpha1.MachineDeployment, useOSM bool) error {
	providerConfig, err := providerconfigtypes.GetConfig(md.Spec.Template.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to read MachineDeployment.Spec.Template.Spec.ProviderSpec: %v", err)
	}

	if useOSM {
		err = ensureOSPAnnotation(md, *providerConfig)
		if err != nil {
			return err
		}
	}

	// Packet has been renamed to Equinix Metal
	if providerConfig.CloudProvider == cloudProviderPacket {
		err = migrateToEquinixMetal(providerConfig)
		if err != nil {
			return fmt.Errorf("failed to migrate packet to equinix metal: %v", err)
		}
	}

	// Update value in original object
	md.Spec.Template.Spec.ProviderSpec.Value.Raw, err = json.Marshal(providerConfig)
	if err != nil {
		return fmt.Errorf("failed to json marshal machine.spec.providerSpec: %v", err)
	}

	return nil
}

func ensureOSPAnnotation(md *v1alpha1.MachineDeployment, providerConfig providerconfigtypes.Config) error {
	// Check for existing annotation
	if _, ok := md.Annotations[osmresources.MachineDeploymentOSPAnnotation]; !ok {
		if md.Annotations == nil {
			md.Annotations = make(map[string]string)
		}
		// Annotation not specified, populate default OSP annotation
		switch providerConfig.OperatingSystem {
		case providerconfigtypes.OperatingSystemUbuntu, providerconfigtypes.OperatingSystemCentOS, providerconfigtypes.OperatingSystemFlatcar,
			providerconfigtypes.OperatingSystemAmazonLinux2:
			md.Annotations[osmresources.MachineDeploymentOSPAnnotation] = fmt.Sprintf(ospNamePattern, providerConfig.OperatingSystem)
			return nil

		default:
			return fmt.Errorf("failed to populate OSP annotation for machinedeployment with unsupported Operating System %s", providerConfig.OperatingSystem)
		}
	}
	return nil
}
