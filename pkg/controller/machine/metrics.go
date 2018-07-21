package controller

import (
	"github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/labels"
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
			[]string{"kubelet_version"}, nil,
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

	machinesCountByKubeletVersion := make(map[string]uint)

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

		machinesCountByKubeletVersion[machine.Spec.Versions.Kubelet]++;
	}

	for version, count := range machinesCountByKubeletVersion {
		ch <- prometheus.MustNewConstMetric(
			mc.machines,
			prometheus.GaugeValue,
			float64(count),
			version,
		)
	}
}
