/*
Copyright 2022 The Machine Controller Authors.

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

	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
	providerconfigtypes "k8c.io/machine-controller/sdk/providerconfig"
)

type contextKey byte

const machineReconcileContextKey contextKey = 0

type reconcileContext struct {
	Machine        *clusterv1alpha1.Machine
	Status         *anxtypes.ProviderStatus
	UserData       string
	Config         resolvedConfig
	ProviderData   *cloudprovidertypes.ProviderData
	ProviderConfig *providerconfigtypes.Config
}

func createReconcileContext(ctx context.Context, cc reconcileContext) context.Context {
	return context.WithValue(ctx, machineReconcileContextKey, cc)
}

func getReconcileContext(ctx context.Context) reconcileContext {
	rawContext := ctx.Value(machineReconcileContextKey)
	if recContext, ok := rawContext.(reconcileContext); ok {
		return recContext
	}

	return reconcileContext{}
}
