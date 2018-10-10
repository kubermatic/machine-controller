package eviction

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// Unfortunatelly we can not directly test `EvictNode` as a List with a fieldSelector
// against a fake client returns nothing
func TestEvictPods(t *testing.T) {
	tests := []struct {
		Name          string
		Pods          []runtime.Object
		OutputObjects []runtime.Object
	}{
		{
			Name: "TestEvictionsGetCreated",
			Pods: []runtime.Object{
				// The subresource actions in the fakeclient do not contain the name
				// of the actual object anymore but its namespace, hence we test if
				// the correct evictions were created by comparing the namespaces
				// => The namespaces of the pods here _must_ differ
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "n1", Name: "pod1"},
					Spec: corev1.PodSpec{NodeName: "node1"}},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "n2", Name: "pod2"},
					Spec: corev1.PodSpec{NodeName: "node1"}},
			},
		},
	}

	for _, test := range tests {
		var literalPods []corev1.Pod
		for _, pod := range test.Pods {
			literalPods = append(literalPods, *(pod.(*corev1.Pod)))
		}
		client := kubefake.NewSimpleClientset(test.Pods...)

		t.Run(test.Name, func(t *testing.T) {

			ne := &NodeEviction{client: client, nodeName: "node1"}
			if errs := ne.evictPods(literalPods); len(errs) > 0 {
				t.Fatalf("Got unexpected errors=%v when running evictPods", errs)
			}

			actions := client.Actions()
			for _, pod := range literalPods {
				hasEviction := false
				for _, action := range actions {
					if action.GetSubresource() == "eviction" && action.GetNamespace() == pod.Namespace {
						hasEviction = true
					}
				}
				if !hasEviction {
					t.Errorf("Did not find expected eviction for pod %s/%s", pod.Namespace, pod.Name)
				}
			}
		})
	}
}
