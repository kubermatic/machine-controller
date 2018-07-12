package controller

import (
	"github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers/core/v1"
)

const metricsPrefix = "machine_controller_"

// NewMachineControllerMetrics creates new MachineControllerMetrics
// with default values initialized, so metrics always show up.
func NewMachineControllerMetrics() *MetricsCollection {
	cm := &MetricsCollection{
		Workers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: metricsPrefix + "workers",
			Help: "The number of running machine controller workers",
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: metricsPrefix + "errors_total",
			Help: "The total number or unexpected errors the controller encountered",
		}),
	}

	// Set default values, so that these metrics always show up
	cm.Workers.Set(0)
	cm.Errors.Add(0)

	return cm
}

type MachineCollector struct {
	lister v1alpha1.MachineLister

	machines       *prometheus.Desc
	machineCreated *prometheus.Desc
	machineDeleted *prometheus.Desc
}

func NewMachineCollector(lister v1alpha1.MachineLister) *MachineCollector {
	return &MachineCollector{
		lister: lister,

		machines: prometheus.NewDesc(
			metricsPrefix+"machines",
			"The number of machines managed by this machine controller",
			nil, nil,
		),
		machineCreated: prometheus.NewDesc(
			metricsPrefix+"machine_created",
			"Timestamp of the machine's creation time",
			[]string{"machine"}, nil,
		),
		machineDeleted: prometheus.NewDesc(
			metricsPrefix+"machine_deleted",
			"Timestamp of the machine's deletion time",
			[]string{"machine"}, nil,
		),
	}
}

func (mc MachineCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mc.machines
	ch <- mc.machineCreated
	ch <- mc.machineDeleted
}

func (mc MachineCollector) Collect(ch chan<- prometheus.Metric) {
	machines, err := mc.lister.List(labels.Everything())
	if err != nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		mc.machines,
		prometheus.GaugeValue,
		float64(len(machines)),
	)

	for _, machine := range machines {
		ch <- prometheus.MustNewConstMetric(
			mc.machineCreated,
			prometheus.GaugeValue,
			float64(machine.CreationTimestamp.Unix()),
			machine.Name,
		)

		if machine.DeletionTimestamp != nil {
			ch <- prometheus.MustNewConstMetric(
				mc.machineDeleted,
				prometheus.GaugeValue,
				float64(machine.DeletionTimestamp.Unix()),
				machine.Name,
			)
		}
	}
}

type NodeController struct {
	lister v1.NodeLister

	nodes       *prometheus.Desc
	nodeCreated *prometheus.Desc
	nodeDeleted *prometheus.Desc
}

func NewNodeController(lister v1.NodeLister) *NodeController {
	return &NodeController{
		lister: lister,

		nodes: prometheus.NewDesc(
			metricsPrefix+"nodes",
			"The number of nodes created by a machine",
			nil, nil,
		),
		nodeCreated: prometheus.NewDesc(
			metricsPrefix+"node_created",
			"The number of nodes created by a machine",
			[]string{"node"}, nil,
		),
		nodeDeleted: prometheus.NewDesc(
			metricsPrefix+"node_deleted",
			"The number of nodes created by a machine",
			[]string{"node"}, nil,
		),
	}
}

func (nc *NodeController) Describe(ch chan<- *prometheus.Desc) {
	ch <- nc.nodes
}

func (nc *NodeController) Collect(ch chan<- prometheus.Metric) {
	nodes, err := nc.lister.List(labels.Everything())
	if err != nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		nc.nodes,
		prometheus.GaugeValue,
		float64(len(nodes)),
	)

	for _, node := range nodes {
		ch <- prometheus.MustNewConstMetric(
			nc.nodeCreated,
			prometheus.GaugeValue,
			float64(node.CreationTimestamp.Unix()),
			node.Name,
		)

		if node.DeletionTimestamp != nil {
			ch <- prometheus.MustNewConstMetric(
				nc.nodeDeleted,
				prometheus.GaugeValue,
				float64(node.DeletionTimestamp.Unix()),
				node.Name,
			)
		}
	}
}
