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

package utils

import (
	"context"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
)

type contextKey byte

const MachineReconcileContextKey contextKey = 0

type ReconcileContext struct {
	Machine      *v1alpha1.Machine
	Status       *anxtypes.ProviderStatus
	UserData     string
	Config       *anxtypes.Config
	ProviderData *cloudprovidertypes.ProviderData
}

func CreateReconcileContext(cc ReconcileContext) context.Context {
	return context.WithValue(context.Background(), MachineReconcileContextKey, cc)
}

func GetReconcileContext(ctx context.Context) ReconcileContext {
	rawContext := ctx.Value(MachineReconcileContextKey)
	if recContext, ok := rawContext.(ReconcileContext); ok {
		return recContext
	}
	return ReconcileContext{}
}
