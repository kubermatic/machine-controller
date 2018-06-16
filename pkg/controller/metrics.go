package controller

import (
	"github.com/prometheus/client_golang/prometheus"
)

// NewMachineControllerMetrics creates new MachineControllerMetrics
// with default values initialized, so metrics always show up.
func NewMachineControllerMetrics() *MetricsCollection {
	namespace := "machine"
	subsystem := "controller"

	cm := &MetricsCollection{
		Machines: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "machines",
			Help:      "The number of machines",
		}),
		Workers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "workers",
			Help:      "The number of running machine controller workers",
		}),
		Nodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "nodes",
			Help:      "The number of nodes created by a machine",
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "The total number or unexpected errors the controller encountered",
		}),
		ControllerOperation: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "controller_operation_duration_seconds",
			Help:      "The duration it takes to execute an operation",
		}, []string{"operation"}),
		NodeJoinDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "node_join_duration_seconds",
			Help:      "The time it takes from creation of the machine resource and the final creation of the node resource",
		}, []string{}),
	}

	// Set default values, so that these metrics always show up
	cm.Machines.Set(0)
	cm.Workers.Set(0)
	cm.Nodes.Set(0)

	return cm
}
