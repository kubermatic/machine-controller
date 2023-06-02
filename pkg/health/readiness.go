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

package health

import (
	"errors"
	"fmt"
	"net/http"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func ApiserverReachable(client kubernetes.Interface) healthz.Checker {
	return func(req *http.Request) error {
		_, err := client.CoreV1().Nodes().List(req.Context(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list nodes check: %w", err)
		}

		return nil
	}
}

func KubeconfigAvailable(kubeconfigProvider machinecontroller.KubeconfigProvider) healthz.Checker {
	return func(req *http.Request) error {
		cm, err := kubeconfigProvider.GetKubeconfig(req.Context())
		if err != nil {
			return fmt.Errorf("unable to get kubeconfig: %w", err)
		}

		if len(cm.Clusters) != 1 {
			return errors.New("invalid kubeconfig: no clusters found")
		}

		for name, c := range cm.Clusters {
			if len(c.CertificateAuthorityData) == 0 {
				return fmt.Errorf("invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.'%s'", name)
			}

			if len(c.Server) == 0 {
				return fmt.Errorf("invalid kubeconfig: no server was specified for kuberconfig.clusters.'%s'", name)
			}
		}

		return nil
	}
}
