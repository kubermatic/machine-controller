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
	"github.com/heptiolabs/healthcheck"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

func ApiserverReachable(client kubernetes.Interface) healthcheck.Check {
	return func() error {
		_, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			klog.V(2).Infof("[healthcheck] Unable to list nodes check: %v", err)
		}
		return err
	}
}
