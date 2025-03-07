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
	"context"
	"encoding/json"
	"fmt"

	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"

	admissionv1 "k8s.io/api/admission/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (ad *admissionData) mutateMachineDeployments(ctx context.Context, ar admissionv1.AdmissionRequest) (*admissionv1.AdmissionResponse, error) {
	machineDeployment := clusterv1alpha1.MachineDeployment{}
	if err := json.Unmarshal(ar.Object.Raw, &machineDeployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}
	machineDeploymentOriginal := machineDeployment.DeepCopy()

	log := ad.log.With("machinedeployment", ctrlruntimeclient.ObjectKeyFromObject(&machineDeployment))
	log.Debug("Defaulting and validating machine deployment")

	machineDeploymentDefaultingFunction(&machineDeployment)

	if err := mutationsForMachineDeployment(&machineDeployment); err != nil {
		return nil, fmt.Errorf("mutation failed: %w", err)
	}

	if errs := validateMachineDeployment(machineDeployment); len(errs) > 0 {
		return nil, fmt.Errorf("validation failed: %v", errs)
	}

	// Do not validate the spec if it hasn't changed
	machineSpecNeedsValidation := true
	if ar.Operation == admissionv1.Update {
		var oldMachineDeployment clusterv1alpha1.MachineDeployment
		if err := json.Unmarshal(ar.OldObject.Raw, &oldMachineDeployment); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OldObject: %w", err)
		}
		if equal := apiequality.Semantic.DeepEqual(oldMachineDeployment.Spec.Template.Spec, machineDeployment.Spec.Template.Spec); equal {
			machineSpecNeedsValidation = false
		}
	}

	if machineSpecNeedsValidation {
		if err := ad.defaultAndValidateMachineSpec(ctx, &machineDeployment.Spec.Template.Spec); err != nil {
			return nil, err
		}
	}

	return createAdmissionResponse(log, machineDeploymentOriginal, &machineDeployment)
}
