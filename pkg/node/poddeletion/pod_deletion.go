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

	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/node/nodemanager"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errorQueueLen = 100
)

type NodeVolumeAttachmentsCleanup struct {
	nodeManager *nodemanager.NodeManager
	nodeName    string
	kubeClient  kubernetes.Interface
}

// New returns a new NodeVolumeAttachmentsCleanup.
func New(nodeName string, client ctrlruntimeclient.Client, kubeClient kubernetes.Interface) *NodeVolumeAttachmentsCleanup {
	return &NodeVolumeAttachmentsCleanup{
		nodeManager: nodemanager.New(client, nodeName),
		nodeName:    nodeName,
		kubeClient:  kubeClient,
	}
}

// Run executes the pod deletion.
func (vc *NodeVolumeAttachmentsCleanup) Run(ctx context.Context, log *zap.SugaredLogger) (bool, bool, error) {
	node, err := vc.nodeManager.GetNode(ctx)
	if err != nil {
		return false, false, fmt.Errorf("failed to get node from lister: %w", err)
	}

	nodeLog := log.With("node", vc.nodeName)
	nodeLog.Info("Starting to cleanup node...")

	// if there are no more volumeAttachments related to the node, then it can be deleted.
	volumeAttachmentsDeleted, err := vc.nodeCanBeDeleted(ctx, nodeLog)
	if err != nil {
		return false, false, fmt.Errorf("failed to check volumeAttachments deletion: %w", err)
	}
	if volumeAttachmentsDeleted {
		return false, true, nil
	}

	// cordon the node to be sure that the deleted pods are re-scheduled in the same node.
	if err := vc.nodeManager.CordonNode(ctx, node); err != nil {
		return false, false, fmt.Errorf("failed to cordon node %s: %w", vc.nodeName, err)
	}
	nodeLog.Debug("Successfully cordoned node.")

	// get all the pods that needs to be deleted (i.e. those mounting volumes attached to the node that is going to be deleted).
	podsToDelete, errors := vc.getFilteredPods(ctx)
	if len(errors) > 0 {
		return false, false, fmt.Errorf("failed to get Pods to delete for node %s, errors encountered: %w", vc.nodeName, err)
	}
	nodeLog.Debugf("Found %d pods to delete for node", len(podsToDelete))

	if len(podsToDelete) == 0 {
		return false, false, nil
	}

	// delete the previously filtered pods, then tells the controller to retry later.
	if errs := vc.deletePods(ctx, nodeLog, podsToDelete); len(errs) > 0 {
		return false, false, fmt.Errorf("failed to delete pods, errors encountered: %v", errs)
	}
	nodeLog.Debug("Successfully deleted all pods mounting persistent volumes attached on node")
	return true, false, err
}

func (vc *NodeVolumeAttachmentsCleanup) getFilteredPods(ctx context.Context) ([]corev1.Pod, []error) {
	filteredPods := []corev1.Pod{}
	lock := sync.Mutex{}
	retErrs := []error{}

	volumeAttachments, err := vc.kubeClient.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		retErrs = append(retErrs, fmt.Errorf("failed to list pods: %w", err))
		return nil, retErrs
	}

	persistentVolumeClaims, err := vc.kubeClient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		retErrs = append(retErrs, fmt.Errorf("failed to list persistent volumes: %w", err))
		return nil, retErrs
	}

	errCh := make(chan error, errorQueueLen)
	wg := sync.WaitGroup{}
	for _, va := range volumeAttachments.Items {
		if va.Spec.NodeName == vc.nodeName {
			for _, pvc := range persistentVolumeClaims.Items {
				if va.Spec.Source.PersistentVolumeName != nil && *va.Spec.Source.PersistentVolumeName == pvc.Spec.VolumeName {
					wg.Add(1)
					go func(pvc corev1.PersistentVolumeClaim) {
						defer wg.Done()
						pods, err := vc.kubeClient.CoreV1().Pods(pvc.Namespace).List(ctx, metav1.ListOptions{})
						switch {
						case kerrors.IsTooManyRequests(err):
							return
						case err != nil:
							errCh <- fmt.Errorf("failed to list pod: %w", err)
						default:
							for _, pod := range pods.Items {
								if doesPodClaimVolume(pod, pvc.Name) && pod.Spec.NodeName == vc.nodeName {
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

// nodeCanBeDeleted checks if all the volumeAttachments related to the node have already been collected by the external CSI driver.
func (vc *NodeVolumeAttachmentsCleanup) nodeCanBeDeleted(ctx context.Context, log *zap.SugaredLogger) (bool, error) {
	volumeAttachments, err := vc.kubeClient.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("error while listing volumeAttachments: %w", err)
	}
	for _, va := range volumeAttachments.Items {
		if va.Spec.NodeName == vc.nodeName {
			log.Infow("Waiting for VolumeAttachment to be deleted before deleting node", "volumeattachment", va.Name)
			return false, nil
		}
	}
	return true, nil
}

func (vc *NodeVolumeAttachmentsCleanup) deletePods(ctx context.Context, log *zap.SugaredLogger, pods []corev1.Pod) []error {
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
				err := vc.kubeClient.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{})
				if err == nil || kerrors.IsNotFound(err) {
					log.Debugw("Successfully deleted pod on node", "pod", ctrlruntimeclient.ObjectKeyFromObject(&p))
					return
				} else if kerrors.IsTooManyRequests(err) {
					// PDB prevents pod deletion, return and make the controller retry later.
					return
				}
				errCh <- fmt.Errorf("error deleting pod %s/%s on node %s: %w", p.Namespace, p.Name, vc.nodeName, err)
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

// doesPodClaimVolume checks if the volume is mounted by the pod.
func doesPodClaimVolume(pod corev1.Pod, pvcName string) bool {
	for _, volumeMount := range pod.Spec.Volumes {
		if volumeMount.PersistentVolumeClaim != nil && volumeMount.PersistentVolumeClaim.ClaimName == pvcName {
			return true
		}
	}
	return false
}
