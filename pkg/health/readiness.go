package health

import (
	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ApiserverReachable(client kubernetes.Interface) healthcheck.Check {
	return func() error {
		_, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			glog.V(2).Infof("[healthcheck] Unable to list nodes check: %v", err)
		}
		return err
	}
}
