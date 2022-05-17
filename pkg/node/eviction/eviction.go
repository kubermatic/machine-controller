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

	evictiontypes "github.com/kubermatic/machine-controller/pkg/node/eviction/types"
	"github.com/kubermatic/machine-controller/pkg/node/nodemanager"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeEviction struct {
	nodeManager *nodemanager.NodeManager
	ctx         context.Context
	nodeName    string
	kubeClient  kubernetes.Interface
}

// New returns a new NodeEviction.
func New(ctx context.Context, nodeName string, client ctrlruntimeclient.Client, kubeClient kubernetes.Interface) *NodeEviction {
	return &NodeEviction{
		nodeManager: nodemanager.New(ctx, client, nodeName),
		ctx:         ctx,
		nodeName:    nodeName,
		kubeClient:  kubeClient,
	}
}

// Run executes the eviction.
func (ne *NodeEviction) Run() (bool, error) {
	node, err := ne.nodeManager.GetNode()
	if err != nil {
		return false, fmt.Errorf("failed to get node from lister: %w", err)
	}
	if _, exists := node.Annotations[evictiontypes.SkipEvictionAnnotationKey]; exists {
		klog.V(3).Infof("Skipping eviction for node %s as it has a %s annotation", ne.nodeName, evictiontypes.SkipEvictionAnnotationKey)
		return false, nil
	}
	klog.V(3).Infof("Starting to evict node %s", ne.nodeName)

	if err := ne.nodeManager.CordonNode(node); err != nil {
		return false, fmt.Errorf("failed to cordon node %s: %w", ne.nodeName, err)
	}
	klog.V(6).Infof("Successfully cordoned node %s", ne.nodeName)

	podsToEvict, err := ne.getFilteredPods()
	if err != nil {
		return false, fmt.Errorf("failed to get Pods to evict for node %s: %w", ne.nodeName, err)
	}
	klog.V(6).Infof("Found %v pods to evict for node %s", len(podsToEvict), ne.nodeName)

	if len(podsToEvict) == 0 {
		return false, nil
	}

	// If we arrived here we have pods to evict, so tell the controller to retry later
	if errs := ne.evictPods(podsToEvict); len(errs) > 0 {
		return true, fmt.Errorf("failed to evict pods, errors encountered: %v", errs)
	}
	klog.V(6).Infof("Successfully created evictions for all pods on node %s!", ne.nodeName)

	return true, nil
}

func (ne *NodeEviction) getFilteredPods() ([]corev1.Pod, error) {
	// The lister-backed client from the mgr automatically creates a lister for all objects requested through it.
	// We explicitly do not want that for pods, hence we have to use the kubernetes core client
	// TODO @alvaroaleman: Add source code ref for this
	pods, err := ne.kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(ne.ctx, metav1.ListOptions{
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

func (ne *NodeEviction) evictPods(pods []corev1.Pod) []error {
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
				err := ne.evictPod(&p)
				if err == nil || kerrors.IsNotFound(err) {
					klog.V(6).Infof("Successfully evicted pod %s/%s on node %s", p.Namespace, p.Name, ne.nodeName)
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
		klog.V(6).Infof("All goroutines for eviction pods on node %s finished", ne.nodeName)
		break
	case err := <-errCh:
		klog.V(6).Infof("Got an error from eviction goroutine for node %s: %v", ne.nodeName, err)
		retErrs = append(retErrs, err)
	}

	return retErrs
}

func (ne *NodeEviction) evictPod(pod *corev1.Pod) error {
	eviction := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return ne.kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(ne.ctx, eviction)
}
