package main

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
)

// MachineControllerMetrics is a struct of all metrics used in
// the machine controller.
type MachineControllerMetrics struct {
	Machines            metrics.Gauge
	Nodes               metrics.Gauge
	Workers             metrics.Gauge
	Errors              metrics.Counter
	ControllerOperation metrics.Histogram
	NodeJoinDuration    metrics.Histogram
}

// NewMachineControllerMetrics creates new MachineControllerMetrics
// with default values initialized, so metrics always show up.
func NewMachineControllerMetrics() *MachineControllerMetrics {
	namespace := "machine"
	subsystem := "controller"

	cm := &MachineControllerMetrics{
		Machines: prometheus.NewGaugeFrom(prom.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "machines",
			Help:      "The number of machines",
		}, []string{}),
		Workers: prometheus.NewGaugeFrom(prom.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "workers",
			Help:      "The number of running machine controller workers",
		}, []string{}),
		Nodes: prometheus.NewGaugeFrom(prom.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "nodes",
			Help:      "The number of nodes created by a machine",
		}, []string{}),
		Errors: prometheus.NewCounterFrom(prom.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "The total number or unexpected errors the controller encountered",
		}, []string{}),
		ControllerOperation: prometheus.NewHistogramFrom(prom.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "controller_operation_duration_seconds",
			Help:      "The duration it takes to execute an operation",
		}, []string{"operation"}),
		NodeJoinDuration: prometheus.NewHistogramFrom(prom.HistogramOpts{
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
