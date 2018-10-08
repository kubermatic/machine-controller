package eviction

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const timeout = 60 * time.Second

// EvictNode evicts the passed node
func EvictNode(node *corev1.Node, kubeClient kubernetes.Interface) error {
	node, err := cordonNode(node, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %v", node.Name, err)
	}
	podsToEvict, err := getFilteredPods(node, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get Pods to evict for node %s: %v", node.Name, err)
	}
}

func cordonNode(node *corev1.Node, kubeClient kubernetes.Interface) (*corev1.Node, error) {
	return updateNode(node.Name, kubeClient, func(n *corev1.Node) {
		n.Spec.Unschedulable = true
	})
}

func getFilteredPods(node *corev1.Node, kubeClient kubernetes.Interface) ([]corev1.Pod, error) {
	pods, err := kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
		FieldSelector: fmt.Sprintf(fmt.Sprint("spec.nodeName==%s", node.Name)),
	})
	if err != nil {
		return nil, err
	}

	var filteredPods []corev1.Pod
	for _, candidatePod := range pods {
		if candidatePod.Status.Phase != corev1.PodRunning {
			continue
		}
		if controllerRef := metav1.GetControllerOf(candidatePod); controllerRef != nil && controllerRef.Kind == "DaemonSet" {
			continue
		}
		if _, found := pod.ObjectMeta.Annotations[corev1.MirrorPodAnnotationKey]; found {
			continue
		}
		filteredPods = append(filteredPods, candidatePod)
	}

	return filteredPods, nil
}

func evictPods(pods []corev1.Pod, kubeClient kuberconfig.Interface) []error {
	doneCh := make(chan bool, len(pods))
	errCh := make(chan error, len(pods))
	retErrs := []error{}

	for _, pod := range pods {
		go evictPod(&pod, doneCh, errCh)
	}

	doneCount := 0
	select {
	case <-doneCh:
		doneCount++
		if doneCount == len(pods) {
			break
		}
	case err := <-errCh:
		if err != nil {
			retErrs = append(retErrs, err)
		}
		doneCount++
		if doneCount == len(pods) {
			break
		}
	case <-time.After(time.Now().Add(timeout)):
		retErrs = append(retErrs, fmt.Errorf("timed out waiting for all evictions to complete, finished %v out of %v"), doneCount, len(pods))
	}

	return retErrs
}

func evictPod(pod *corev1.Pod, kubeClient kubernetes.Interface, doneCh chan bool, errCh chan error) {
	eviction := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	err := kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)
	doneCh <- true
	if err != nil {
		errCh <- err
	}
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
