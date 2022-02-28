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

package poddeletion

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrorQueueLen = 1000
)

type NodeVolumeAttachmentsCleanup struct {
	ctx        context.Context
	nodeName   string
	client     ctrlruntimeclient.Client
	kubeClient kubernetes.Interface
}

// New returns a new NodeVolumeAttachmentsCleanup
func New(ctx context.Context, nodeName string, client ctrlruntimeclient.Client, kubeClient kubernetes.Interface) *NodeVolumeAttachmentsCleanup {
	return &NodeVolumeAttachmentsCleanup{
		ctx:        ctx,
		nodeName:   nodeName,
		client:     client,
		kubeClient: kubeClient,
	}
}

// Run executes the eviction
func (vc *NodeVolumeAttachmentsCleanup) Run() (bool, bool, error) {
	node, err := vc.getNode()
	if err != nil {
		return false, false, fmt.Errorf("failed to get node from lister: %v", err)
	}
	klog.V(3).Infof("Starting to cleanup node %s", vc.nodeName)

	volumeAttachmentsDeleted, err := vc.nodeCanBeDeleted()
	if err != nil {
		return false, false, fmt.Errorf("failed to check volumeAttachments deletion: %v", err)
	}
	if volumeAttachmentsDeleted {
		return false, true, nil
	}

	if err := vc.CordonNode(node); err != nil {
		return false, false, fmt.Errorf("failed to cordon node %s: %v", vc.nodeName, err)
	}
	klog.V(6).Infof("Successfully cordoned node %s", vc.nodeName)

	podsToDelete, errors := vc.getFilteredPods()
	if len(errors) > 0 {
		return false, false, fmt.Errorf("failed to get Pods to delete for node %s, errors encountered: %v", vc.nodeName, err)
	}
	klog.V(6).Infof("Found %v pods to delete for node %s", len(podsToDelete), vc.nodeName)

	if len(podsToDelete) == 0 {
		return false, false, nil
	}

	// If we arrived here we have pods to evict, so tell the controller to retry later
	if errs := vc.deletePods(podsToDelete); len(errs) > 0 {
		return false, false, fmt.Errorf("failed to delete pods, errors encountered: %v", errs)
	}
	klog.V(6).Infof("Successfully deleted all pods mounting persistent volumes on node %s", vc.nodeName)
	return true, false, err
}

func (vc *NodeVolumeAttachmentsCleanup) getNode() (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := vc.client.Get(vc.ctx, types.NamespacedName{Name: vc.nodeName}, node); err != nil {
		return nil, fmt.Errorf("failed to get node from lister: %v", err)
	}
	return node, nil
}

func (vc *NodeVolumeAttachmentsCleanup) CordonNode(node *corev1.Node) error {
	if !node.Spec.Unschedulable {
		_, err := vc.updateNode(func(n *corev1.Node) {
			n.Spec.Unschedulable = true
		})
		if err != nil {
			return err
		}
	}

	// Be paranoid and wait until the change got propagated to the lister
	// This assumes that the delay between our lister and the APIserver
	// is smaller or equal to the delay the schedulers lister has - If
	// that is not the case, there is a small chance the scheduler schedules
	// pods in between, those will then get deleted upon node deletion and
	// not evicted
	return wait.Poll(1*time.Second, 10*time.Second, func() (bool, error) {
		node := &corev1.Node{}
		if err := vc.client.Get(vc.ctx, types.NamespacedName{Name: vc.nodeName}, node); err != nil {
			return false, err
		}
		if node.Spec.Unschedulable {
			return true, nil
		}
		return false, nil
	})
}

func (vc *NodeVolumeAttachmentsCleanup) getFilteredPods() ([]corev1.Pod, []error) {
	filteredPods := []corev1.Pod{}
	lock := sync.Mutex{}
	retErrs := []error{}

	volumeAttachments, err := vc.kubeClient.StorageV1().VolumeAttachments().List(vc.ctx, metav1.ListOptions{})
	if err != nil {
		retErrs = append(retErrs, fmt.Errorf("failed to list pods: %v", err))
		return nil, retErrs
	}

	persistentVolumeClaims, err := vc.kubeClient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(vc.ctx, metav1.ListOptions{})
	if err != nil {
		retErrs = append(retErrs, fmt.Errorf("failed to list persistent volumes: %v", err))
		return nil, retErrs
	}

	errCh := make(chan error, ErrorQueueLen)
	wg := sync.WaitGroup{}
	for _, va := range volumeAttachments.Items {
		if va.Spec.NodeName == vc.nodeName {
			for _, pvc := range persistentVolumeClaims.Items {
				if va.Spec.Source.PersistentVolumeName != nil && *va.Spec.Source.PersistentVolumeName == pvc.Spec.VolumeName {
					wg.Add(1)
					go func(pvc corev1.PersistentVolumeClaim) {
						defer wg.Done()
						pods, err := vc.kubeClient.CoreV1().Pods(pvc.Namespace).List(vc.ctx, metav1.ListOptions{})
						switch {
						case kerrors.IsTooManyRequests(err):
							return
						case err != nil:
							errCh <- fmt.Errorf("failed to list pod: %v", err)
						default:
							for _, pod := range pods.Items {
								if doesPodClaimVolume(pod, pvc.Name) {
									lock.Lock()
									filteredPods = append(filteredPods, pod)
									lock.Unlock()
								}
							}
						}
					}(pvc)
				}
			}
		}
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		retErrs = append(retErrs, err)
	}

	return filteredPods, nil
}

// doesPodClaimVolume checks if the volume is mounted by the pod
func doesPodClaimVolume(pod corev1.Pod, pvcName string) bool {
	for _, volumeMount := range pod.Spec.Volumes {
		if volumeMount.PersistentVolumeClaim != nil && volumeMount.PersistentVolumeClaim.ClaimName == pvcName {
			return true
		}
	}
	return false
}

// nodeCanBeDeleted checks if all the volumeAttachments related to the node have already been collected by the external CSI driver
func (vc *NodeVolumeAttachmentsCleanup) nodeCanBeDeleted() (bool, error) {
	volumeAttachments, err := vc.kubeClient.StorageV1().VolumeAttachments().List(vc.ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("error while listing volumeAttachments: %v", err)
	}
	for _, va := range volumeAttachments.Items {
		if va.Spec.NodeName == vc.nodeName {
			klog.V(3).Infof("waiting for the volumeAttachment %s to be deleted before deleting node %s", va.Name, vc.nodeName)
			return false, nil
		}
	}
	return true, nil
}

func (vc *NodeVolumeAttachmentsCleanup) deletePods(pods []corev1.Pod) []error {

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
				err := vc.kubeClient.CoreV1().Pods(p.Namespace).Delete(vc.ctx, p.Name, metav1.DeleteOptions{})
				if err == nil || kerrors.IsNotFound(err) {
					klog.V(6).Infof("Successfully evicted pod %s/%s on node %s", p.Namespace, p.Name, vc.nodeName)
					return
				} else if kerrors.IsTooManyRequests(err) {
					// PDB prevents eviction, return and make the controller retry later
					return
				} else {
					errCh <- fmt.Errorf("error evicting pod %s/%s on node %s: %v", p.Namespace, p.Name, vc.nodeName, err)
					return
				}
			}
		}(pod)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		retErrs = append(retErrs, err)
	}

	return retErrs
}

func (vc *NodeVolumeAttachmentsCleanup) updateNode(modify func(*corev1.Node)) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := vc.client.Get(vc.ctx, types.NamespacedName{Name: vc.nodeName}, node); err != nil {
			return err
		}
		// Apply modifications
		modify(node)
		// Update the node
		return vc.client.Update(vc.ctx, node)
	})

	return node, err
}
