package health

import (
	"github.com/heptiolabs/healthcheck"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ApiserverReachable(client kubernetes.Interface) healthcheck.Check {
	return func() error {
		_, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
		return err
	}
}
