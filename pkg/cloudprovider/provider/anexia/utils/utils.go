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
