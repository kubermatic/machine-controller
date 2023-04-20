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

package eviction

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	evictiontypes "github.com/kubermatic/machine-controller/pkg/node/eviction/types"
	"github.com/kubermatic/machine-controller/pkg/node/nodemanager"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeEviction struct {
	nodeManager *nodemanager.NodeManager
	nodeName    string
	kubeClient  kubernetes.Interface
}

// New returns a new NodeEviction.
func New(nodeName string, client ctrlruntimeclient.Client, kubeClient kubernetes.Interface) *NodeEviction {
	return &NodeEviction{
		nodeManager: nodemanager.New(client, nodeName),
		nodeName:    nodeName,
		kubeClient:  kubeClient,
	}
}

// Run executes the eviction.
func (ne *NodeEviction) Run(ctx context.Context, log *zap.SugaredLogger) (bool, error) {
	nodeLog := log.With("node", ne.nodeName)

	node, err := ne.nodeManager.GetNode(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get node from lister: %w", err)
	}
	if _, exists := node.Annotations[evictiontypes.SkipEvictionAnnotationKey]; exists {
		nodeLog.Info("Skipping eviction for node as it has a %s annotation", evictiontypes.SkipEvictionAnnotationKey)
		return false, nil
	}

	nodeLog.Info("Starting to evict node")

	if err := ne.nodeManager.CordonNode(ctx, node); err != nil {
		return false, fmt.Errorf("failed to cordon node %s: %w", ne.nodeName, err)
	}
	nodeLog.Debug("Successfully cordoned node")

	podsToEvict, err := ne.getFilteredPods(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get Pods to evict for node %s: %w", ne.nodeName, err)
	}
	nodeLog.Debugf("Found %d pods to evict for node", len(podsToEvict))

	if len(podsToEvict) == 0 {
		return false, nil
	}

	// If we arrived here we have pods to evict, so tell the controller to retry later
	if errs := ne.evictPods(ctx, nodeLog, podsToEvict); len(errs) > 0 {
		return true, fmt.Errorf("failed to evict pods, errors encountered: %v", errs)
	}
	nodeLog.Debug("Successfully created evictions for all pods on node")

	return true, nil
}

func (ne *NodeEviction) getFilteredPods(ctx context.Context) ([]corev1.Pod, error) {
	// The lister-backed client from the mgr automatically creates a lister for all objects requested through it.
	// We explicitly do not want that for pods, hence we have to use the kubernetes core client
	// TODO @alvaroaleman: Add source code ref for this
	pods, err := ne.kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": ne.nodeName}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var filteredPods []corev1.Pod
	for _, candidatePod := range pods.Items {
		if candidatePod.Status.Phase == corev1.PodSucceeded || candidatePod.Status.Phase == corev1.PodFailed {
			continue
		}
		if controllerRef := metav1.GetControllerOf(&candidatePod); controllerRef != nil && controllerRef.Kind == "DaemonSet" {
			continue
		}
		if _, found := candidatePod.ObjectMeta.Annotations[corev1.MirrorPodAnnotationKey]; found {
			continue
		}
		filteredPods = append(filteredPods, candidatePod)
	}

	return filteredPods, nil
}

func (ne *NodeEviction) evictPods(ctx context.Context, log *zap.SugaredLogger, pods []corev1.Pod) []error {
	errCh := make(chan error, len(pods))
	retErrs := []error{}

	var wg sync.WaitGroup
	var isDone bool
	defer func() { isDone = true }()

	wg.Add(len(pods))
	for _, pod := range pods {
		go func(p corev1.Pod) {
			defer wg.Done()
			for {
				if isDone {
					return
				}
				err := ne.evictPod(ctx, &p)
				if err == nil || kerrors.IsNotFound(err) {
					log.Debugw("Successfully evicted pod on node", "pod", ctrlruntimeclient.ObjectKeyFromObject(&p))
					return
				} else if kerrors.IsTooManyRequests(err) {
					// PDB prevents eviction, return and make the controller retry later
					return
				} else {
					errCh <- fmt.Errorf("error evicting pod %s/%s on node %s: %w", p.Namespace, p.Name, ne.nodeName, err)
					return
				}
			}
		}(pod)
	}

	finished := make(chan struct{})
	go func() { wg.Wait(); finished <- struct{}{} }()

	select {
	case <-finished:
		log.Debug("All goroutines for eviction pods on node finished")
		break
	case err := <-errCh:
		log.Debugw("Got an error from eviction goroutine for node", zap.Error(err))
		retErrs = append(retErrs, err)
	}

	return retErrs
}

func (ne *NodeEviction) evictPod(ctx context.Context, pod *corev1.Pod) error {
	eviction := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return ne.kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(ctx, eviction)
}
