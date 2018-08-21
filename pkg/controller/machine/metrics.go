package controller

import (
	"fmt"

	"github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
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

// MachineCollector is a Prometheus metrics collector.
type MachineCollector struct {
	machineLister v1alpha1.MachineLister
	nodeLister    listerscorev1.NodeLister
	kubeClient    kubernetes.Interface

	machines       *prometheus.Desc
	machineCreated *prometheus.Desc
	machineDeleted *prometheus.Desc
	nodes          *prometheus.Desc
}

type machineMetricLabels struct {
	KubeletVersion  string
	CloudProvider   providerconfig.CloudProvider
	OperatingSystem providerconfig.OperatingSystem
	ProviderLabels  map[string]string
}

// Counter turns a label collection into a Prometheus counter.
func (l *machineMetricLabels) Counter(value uint) prometheus.Counter {
	labels := make(map[string]string)
	labelNames := make([]string, 0)

	if len(l.KubeletVersion) > 0 {
		labels["kubelet_version"] = l.KubeletVersion
	}

	if len(l.CloudProvider) > 0 {
		labels["provider"] = string(l.CloudProvider)
	}

	if len(l.OperatingSystem) > 0 {
		labels["os"] = string(l.OperatingSystem)
	}

	for k, v := range l.ProviderLabels {
		labels[k] = v
	}

	for k := range labels {
		labelNames = append(labelNames, k)
	}

	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricsPrefix + "machines",
	}, labelNames)

	counter := counterVec.With(labels)
	counter.Set(float64(value))

	return counter
}

type nodeMetricLabels struct {
	ContainerRuntime string
	KubeletVersion   string
	OperatingSystem  string
	OSImage          string
	Architecture     string
}

// Counter turns a label collection into a Prometheus counter.
func (l *nodeMetricLabels) Counter(value uint) prometheus.Counter {
	labels := make(map[string]string)
	labelNames := make([]string, 0)

	if len(l.KubeletVersion) > 0 {
		labels["kubelet_version"] = l.KubeletVersion
	}

	if len(l.ContainerRuntime) > 0 {
		labels["runtime"] = string(l.ContainerRuntime)
	}

	if len(l.OperatingSystem) > 0 {
		labels["os"] = string(l.OperatingSystem)
	}

	if len(l.Architecture) > 0 {
		labels["architecture"] = string(l.Architecture)
	}

	if len(l.OSImage) > 0 {
		labels["osimage"] = string(l.OSImage)
	}

	for k := range labels {
		labelNames = append(labelNames, k)
	}

	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricsPrefix + "nodes",
	}, labelNames)

	counter := counterVec.With(labels)
	counter.Set(float64(value))

	return counter
}

// NewMachineCollector creates a new machine metrics collector.
func NewMachineCollector(machineLister v1alpha1.MachineLister, nodeLister listerscorev1.NodeLister, kubeClient kubernetes.Interface) *MachineCollector {
	return &MachineCollector{
		machineLister: machineLister,
		nodeLister:    nodeLister,
		kubeClient:    kubeClient,

		machines: prometheus.NewDesc(
			metricsPrefix+"machines",
			"The number of machines managed by this machine controller",
			[]string{}, nil,
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
		nodes: prometheus.NewDesc(
			metricsPrefix+"nodes",
			"The number of actually existing Kubernetes nodes",
			[]string{}, nil,
		),
	}
}

// Describe implements the prometheus.Collector interface.
func (mc MachineCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mc.machines
	ch <- mc.machineCreated
	ch <- mc.machineDeleted
	ch <- mc.nodes
}

// Collect implements the prometheus.Collector interface.
func (mc MachineCollector) Collect(ch chan<- prometheus.Metric) {
	machines, err := mc.machineLister.List(labels.Everything())
	if err != nil {
		runtime.HandleError(fmt.Errorf("failed to list machines for machines metric: %v", err))
		return
	}

	cvr := providerconfig.NewConfigVarResolver(mc.kubeClient)
	machineCountByLabels := make(map[*machineMetricLabels]uint)

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

		providerConfig, err := providerconfig.GetConfig(machine.Spec.ProviderConfig)
		if err != nil {
			runtime.HandleError(fmt.Errorf("failed to determine provider config for machine: %v", err))
			continue
		}

		provider, err := cloudprovider.ForProvider(providerConfig.CloudProvider, cvr)
		if err != nil {
			runtime.HandleError(fmt.Errorf("failed to determine provider provider: %v", err))
			continue
		}

		labels, err := provider.MachineMetricsLabels(machine)
		if err != nil {
			runtime.HandleError(fmt.Errorf("failed to determine machine metrics labels: %v", err))
			continue
		}

		metricsLabels := machineMetricLabels{
			KubeletVersion:  machine.Spec.Versions.Kubelet,
			CloudProvider:   providerConfig.CloudProvider,
			OperatingSystem: providerConfig.OperatingSystem,
			ProviderLabels:  labels,
		}

		var key *machineMetricLabels

		for p := range machineCountByLabels {
			if equality.Semantic.DeepEqual(*p, metricsLabels) {
				key = p
				break
			}
		}

		if key == nil {
			key = &metricsLabels
		}

		machineCountByLabels[key]++
	}

	// ensure that we always report at least a machines=0
	if len(machineCountByLabels) == 0 {
		machineCountByLabels[&machineMetricLabels{}] = 0
	}

	for info, count := range machineCountByLabels {
		ch <- info.Counter(count)
	}

	// Gather the same kind of information in much the same
	// way for nodes instead of machines.

	nodes, err := mc.nodeLister.List(labels.Everything())
	if err != nil {
		runtime.HandleError(fmt.Errorf("failed to list nodes for machines metric: %v", err))
		return
	}

	nodeCountByLabels := make(map[*nodeMetricLabels]uint)
	for _, node := range nodes {
		nodeInfo := node.Status.NodeInfo

		metricsLabels := nodeMetricLabels{
			ContainerRuntime: nodeInfo.ContainerRuntimeVersion,
			KubeletVersion:   nodeInfo.KubeletVersion,
			OperatingSystem:  nodeInfo.OperatingSystem,
			OSImage:          nodeInfo.OSImage,
			Architecture:     nodeInfo.Architecture,
		}

		var key *nodeMetricLabels

		for p := range nodeCountByLabels {
			if equality.Semantic.DeepEqual(*p, metricsLabels) {
				key = p
				break
			}
		}

		if key == nil {
			key = &metricsLabels
		}

		nodeCountByLabels[key]++
	}

	// ensure that we always report at least a nodes=0
	if len(nodeCountByLabels) == 0 {
		nodeCountByLabels[&nodeMetricLabels{}] = 0
	}

	for info, count := range nodeCountByLabels {
		ch <- info.Counter(count)
	}
}
