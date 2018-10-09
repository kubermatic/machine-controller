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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const timeout = 60 * time.Second

// EvictNode evicts the passed node
func EvictNode(node *corev1.Node, kubeClient kubernetes.Interface) error {
	glog.V(4).Infof("Starting to evict node %s", node.Name)

	node, err := cordonNode(node, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %v", node.Name, err)
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
	return nil
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
	for _, candidatePod := range pods.Items {
		if candidatePod.Status.Phase != corev1.PodRunning {
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

func evictPods(pods []corev1.Pod, kubeClient kubernetes.Interface) []error {
	if len(pods) == 0 {
		return nil
	}

	errCh := make(chan error, len(pods))
	retErrs := []error{}

	var wg sync.WaitGroup
	var isDone bool
	defer func() { isDone = true }()

	for _, pod := range pods {
		go func() {
			wg.Add(1)
			defer wg.Done()
			for {
				if isDone {
					return
				}
				err := evictPod(&pod, kubeClient)
				if err == nil || kerrors.IsNotFound(err) {
					return
				} else if kerrors.IsTooManyRequests(err) {
					time.Sleep(5 * time.Second)
				} else {
					errCh <- fmt.Errorf("error evicting pod %s/%s: %v", pod.Namespace, pod.Name, err)
					return
				}
			}
		}()
	}

	var finished chan struct{}
	go func() { wg.Wait(); finished <- struct{}{} }()

	select {
	case <-finished:
		break
	case err := <-errCh:
		retErrs = append(retErrs, err)
	case <-time.After(timeout):
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
