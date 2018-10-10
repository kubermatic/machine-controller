package eviction

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	timeout                   = 60 * time.Second
	SkipEvictionAnnotationKey = "kubermatic.io/skip-eviction"
)

// EvictNode evicts the passed node
func EvictNode(node *corev1.Node, kubeClient kubernetes.Interface) error {
	if node.Annotations != nil {
		if _, exists := node.Annotations[SkipEvictionAnnotationKey]; exists {
			glog.V(4).Infof("Skipping eviction for node %s as it has a %s annotation", node.Name, SkipEvictionAnnotationKey)
			return nil
		}
	}

	glog.V(4).Infof("Starting to evict node %s", node.Name)

	// Required to not cause a NPE when passing back the nodeName in the error
	nodeName := node.Name
	node, err := cordonNode(node, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %v", nodeName, err)
	}
	glog.V(4).Infof("Successfully cordoned node %s", node.Name)

	podsToEvict, err := getFilteredPods(node, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get Pods to evict for node %s: %v", node.Name, err)
	}
	glog.V(4).Infof("Found %v pods to evict for node %s", len(podsToEvict), node.Name)

	errs := evictPods(podsToEvict, kubeClient)
	if len(errs) > 0 {
		return fmt.Errorf("failed to evict pods, errors encountered: %v", errs)
	}
	glog.V(4).Infof("Successfully evicted all pods for node %s!", node.Name)

	glog.V(4).Infof("Waiting for deletion of all pods for node %s", nodeName)
	if err := waitForDeletion(podsToEvict, kubeClient); err != nil {
		return fmt.Errorf("failed waiting for pods of node %s to be deleted: %v", nodeName, err)
	}
	glog.V(4).Infof("All pods of node %s were successfully deleted", nodeName)

	return nil
}

func cordonNode(node *corev1.Node, kubeClient kubernetes.Interface) (*corev1.Node, error) {
	nodeName := node.Name
	node, err := updateNode(node.Name, kubeClient, func(n *corev1.Node) {
		n.Spec.Unschedulable = true
	})
	err = wait.Poll(1*time.Second, timeout, func() (bool, error) {
		node, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if node.Spec.Unschedulable {
			return true, nil
		}
		return false, nil
	})
	return node, err
}

func getFilteredPods(node *corev1.Node, kubeClient kubernetes.Interface) ([]corev1.Pod, error) {
	pods, err := kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name}).String(),
	})
	if err != nil {
		return nil, err
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
		glog.V(5).Infof("Appending pod %s/%s for node %s", candidatePod.Namespace, candidatePod.Name, candidatePod.Spec.NodeName)
		filteredPods = append(filteredPods, candidatePod)
	}

	return filteredPods, nil
}

func evictPods(pods []corev1.Pod, kubeClient kubernetes.Interface) []error {
	if len(pods) == 0 {
		return nil
	}
	nodeName := pods[0].Spec.NodeName

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
				err := evictPod(&p, kubeClient)
				if err == nil || kerrors.IsNotFound(err) {
					glog.V(5).Infof("Successfully evicted pod %s/%s on node %s", p.Namespace, p.Name, nodeName)
					return
				} else if kerrors.IsTooManyRequests(err) {
					glog.V(5).Infof("Will retry eviction for pod %s/%s on node %s", p.Namespace, p.Name, nodeName)
					time.Sleep(5 * time.Second)
				} else {
					errCh <- fmt.Errorf("error evicting pod %s/%s: %v", p.Namespace, p.Name, err)
					return
				}
			}
		}(pod)
	}

	finished := make(chan struct{})
	go func() { wg.Wait(); finished <- struct{}{} }()

	select {
	case <-finished:
		glog.V(5).Infof("All goroutines for eviction pods on node %s finished", nodeName)
		break
	case err := <-errCh:
		glog.V(5).Infof("Got an error from eviction goroutine for node %s: %v", nodeName, err)
		retErrs = append(retErrs, err)
	case <-time.After(timeout):
		retErrs = append(retErrs, fmt.Errorf("timed out waiting for evictions to complete"))
		glog.V(5).Infof("Timed out waiting for all evition goroutiness for node %s to finish", nodeName)
		break
	}

	return retErrs
}

func evictPod(pod *corev1.Pod, kubeClient kubernetes.Interface) error {
	eviction := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)
}

func updateNode(name string, client kubernetes.Interface, modify func(*corev1.Node)) (*corev1.Node, error) {
	var updatedNode *corev1.Node
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var retryErr error

		//Get latest version from API
		currentNode, err := client.CoreV1().Nodes().Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		// Apply modifications
		modify(currentNode)
		// Update the node
		updatedNode, retryErr = client.CoreV1().Nodes().Update(currentNode)
		return retryErr
	})

	return updatedNode, err
}

func waitForDeletion(pods []corev1.Pod, kubeClient kubernetes.Interface) error {
	return wait.Poll(1*time.Second, timeout, func() (bool, error) {
		for _, pod := range pods {
			_, err := kubeClient.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
			if err != nil && kerrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return true, nil
	})
}
