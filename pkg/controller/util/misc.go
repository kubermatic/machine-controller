package util

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// EnqueueRequestForObjectExceptDelete helps to ignore delete events for machinedeployments
// and machinesets resources.
// It behaves just like EnqueueRequestForObject except we override Delete func.
// It is useful to solve partial deadlocks due to old state of a parent resource in the cache.
// It occurs when machinedeployment/machineset in cache does not reflect deletion state yet,
// and controller ends up recreating child resources for it.
// It is safe to ignore delete events for machinedeployments and machinesets because deleting child resources
// handled via owner references and there is nothing else to do.
type EnqueueRequestForObjectExceptDelete struct {
	handler.EnqueueRequestForObject
}

func (e *EnqueueRequestForObjectExceptDelete) Delete(_ event.DeleteEvent, _ workqueue.RateLimitingInterface) {
}
