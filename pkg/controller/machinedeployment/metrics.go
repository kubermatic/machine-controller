/*
Copyright 2025 The Machine Controller Authors.

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

package machinedeployment

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const metricsPrefix = "machine_deployment_"

type Collector struct {
	ctx    context.Context
	client ctrlruntimeclient.Client

	replicas          *prometheus.Desc
	availableReplicas *prometheus.Desc
	readyReplicas     *prometheus.Desc
	updatedReplicas   *prometheus.Desc
}

// NewCollector creates new machine deployment collector for metrics collection.
func NewCollector(ctx context.Context, client ctrlruntimeclient.Client) *Collector {
	return &Collector{
		ctx:    ctx,
		client: client,
		replicas: prometheus.NewDesc(
			metricsPrefix+"replicas",
			"The number of replicas defined for a machine deployment",
			[]string{"name", "namespace"}, nil,
		),
		availableReplicas: prometheus.NewDesc(
			metricsPrefix+"available_replicas",
			"The number of available replicas for a machine deployment",
			[]string{"name", "namespace"}, nil,
		),
		readyReplicas: prometheus.NewDesc(
			metricsPrefix+"ready_replicas",
			"The number of ready replicas for a machine deployment",
			[]string{"name", "namespace"}, nil,
		),
		updatedReplicas: prometheus.NewDesc(
			metricsPrefix+"updated_replicas",
			"The number of replicas updated for a machine deployment",
			[]string{"name", "namespace"}, nil,
		),
	}
}

// Describe implements the prometheus.Describe interface.
func (c *Collector) Describe(desc chan<- *prometheus.Desc) {
	desc <- c.replicas
	desc <- c.readyReplicas
	desc <- c.availableReplicas
	desc <- c.readyReplicas
}

// Collect implements the prometheus.Collector interface.
func (c *Collector) Collect(metrics chan<- prometheus.Metric) {
	machineDeployments := &v1alpha1.MachineDeploymentList{}
	if err := c.client.List(c.ctx, machineDeployments); err != nil {
		return
	}

	for _, machineDeployment := range machineDeployments.Items {
		metrics <- prometheus.MustNewConstMetric(
			c.replicas,
			prometheus.GaugeValue,
			float64(machineDeployment.Status.Replicas),
			machineDeployment.Name,
			machineDeployment.Namespace,
		)
		metrics <- prometheus.MustNewConstMetric(
			c.readyReplicas,
			prometheus.GaugeValue,
			float64(machineDeployment.Status.ReadyReplicas),
			machineDeployment.Name,
			machineDeployment.Namespace,
		)
		metrics <- prometheus.MustNewConstMetric(
			c.availableReplicas,
			prometheus.GaugeValue,
			float64(machineDeployment.Status.AvailableReplicas),
			machineDeployment.Name,
			machineDeployment.Namespace,
		)
		metrics <- prometheus.MustNewConstMetric(
			c.updatedReplicas,
			prometheus.GaugeValue,
			float64(machineDeployment.Status.UpdatedReplicas),
			machineDeployment.Name,
			machineDeployment.Namespace,
		)
	}
}
