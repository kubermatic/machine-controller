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
	CloudCreateDuration metrics.Histogram
	CloudDeleteDuration metrics.Histogram
	CloudGetDuration    metrics.Histogram
	ValidateDuration    metrics.Histogram
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
			Help:      "The number of currently managed machines",
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
		CloudCreateDuration: prometheus.NewHistogramFrom(prom.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cloud_create_duration_seconds",
			Help:      "The time it takes to create an instance on the cloud provider",
		}, []string{}),
		CloudDeleteDuration: prometheus.NewHistogramFrom(prom.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cloud_delete_duration_seconds",
			Help:      "The time it takes to delete an instance on the cloud provider",
		}, []string{}),
		CloudGetDuration: prometheus.NewHistogramFrom(prom.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cloud_get_duration_seconds",
			Help:      "The time it takes to get an instance from the cloud provider",
		}, []string{}),
		ValidateDuration: prometheus.NewHistogramFrom(prom.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "validate_duration_seconds",
			Help:      "The time it takes to validate a machine",
		}, []string{}),
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
