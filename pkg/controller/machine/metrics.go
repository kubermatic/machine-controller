/*
Copyright 2019 The Machine Controller Authors.

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

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/api/equality"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	metricsPrefix            = "machine_controller_"
	metricRegatherWaitPeriod = 10 * time.Minute
)

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
	ctx    context.Context
	client ctrlruntimeclient.Client

	machines       *prometheus.Desc
	machineCreated *prometheus.Desc
	machineDeleted *prometheus.Desc
}

type machineMetricLabels struct {
	KubeletVersion  string
	CloudProvider   providerconfigtypes.CloudProvider
	OperatingSystem providerconfigtypes.OperatingSystem
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
		Name: metricsPrefix + "machines_total",
		Help: "Total number of machines",
	}, labelNames)

	counter := counterVec.With(labels)
	counter.Add(float64(value))

	return counter
}

func NewMachineCollector(ctx context.Context, client ctrlruntimeclient.Client) *MachineCollector {
	// Start periodically calling the providers SetMetricsForMachines in a dedicated go routine
	skg := providerconfig.NewConfigVarResolver(ctx, client)
	go func() {
		metricGatheringExecutor := func() {
			machines := &clusterv1alpha1.MachineList{}
			if err := client.List(ctx, machines); err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to list machines for SetMetricsForMachines: %w", err))
				return
			}
			var machineList clusterv1alpha1.MachineList
			for _, machine := range machines.Items {
				machineList.Items = append(machineList.Items, *machine.DeepCopy())
			}
			if len(machineList.Items) < 1 {
				return
			}

			providerMachineMap := map[providerconfigtypes.CloudProvider]*clusterv1alpha1.MachineList{}
			for _, machine := range machines.Items {
				providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
				if err != nil {
					utilruntime.HandleError(fmt.Errorf("failed to get providerSpec for SetMetricsForMachines: %w", err))
					continue
				}
				if _, exists := providerMachineMap[providerConfig.CloudProvider]; !exists {
					providerMachineMap[providerConfig.CloudProvider] = &clusterv1alpha1.MachineList{}
				}
				providerMachineMap[providerConfig.CloudProvider].Items = append(providerMachineMap[providerConfig.CloudProvider].Items, machine)
			}

			for provider, providerMachineList := range providerMachineMap {
				prov, err := cloudprovider.ForProvider(provider, skg)
				if err != nil {
					utilruntime.HandleError(fmt.Errorf("failed to get cloud provider for SetMetricsForMachines:: %q: %w", provider, err))
					continue
				}
				if err := prov.SetMetricsForMachines(*providerMachineList); err != nil {
					utilruntime.HandleError(fmt.Errorf("failed to call prov.SetInstanceNumberForMachines: %w", err))
					continue
				}
			}
		}
		for {
			metricGatheringExecutor()
			time.Sleep(metricRegatherWaitPeriod)
		}
	}()

	return &MachineCollector{
		ctx:    ctx,
		client: client,

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
	}
}

// Describe implements the prometheus.Collector interface.
func (mc MachineCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mc.machines
	ch <- mc.machineCreated
	ch <- mc.machineDeleted
}

// Collect implements the prometheus.Collector interface.
func (mc MachineCollector) Collect(ch chan<- prometheus.Metric) {
	machines := &clusterv1alpha1.MachineList{}
	if err := mc.client.List(mc.ctx, machines); err != nil {
		return
	}

	cvr := providerconfig.NewConfigVarResolver(mc.ctx, mc.client)
	machineCountByLabels := make(map[*machineMetricLabels]uint)

	for _, machine := range machines.Items {
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

		providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to determine providerSpec for machine %s: %w", machine.Name, err))
			continue
		}

		provider, err := cloudprovider.ForProvider(providerConfig.CloudProvider, cvr)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to determine provider provider: %w", err))
			continue
		}

		labels, err := provider.MachineMetricsLabels(&machine)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to determine machine metrics labels: %w", err))
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
}
