package health

import (
	"errors"

	"github.com/heptiolabs/healthcheck"
	"github.com/kubermatic/machine-controller/pkg/machines"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ApiserverReachable(client kubernetes.Interface) healthcheck.Check {
	return func() error {
		_, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
		return err
	}
}

func CustomResourceDefinitionsEstablished(clientset apiextensionsclient.Interface) healthcheck.Check {
	return func() error {
		exist, err := machines.AllCustomResourceDefinitionsExists(clientset)
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("custom resource definitions do not exist / are established")
		}
		return nil
	}
}
