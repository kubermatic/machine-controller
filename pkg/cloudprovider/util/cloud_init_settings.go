/*
Copyright 2021 The Machine Controller Authors.

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

package util

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CloudInitNamespace    = "cloud-init-settings"
	cloudInitGetterSecret = "cloud-init-getter-token"
)

func ExtractTokenAndAPIServer(ctx context.Context, userdata string, client ctrlruntimeclient.Client) (string, string, error) {
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: cloudInitGetterSecret, Namespace: CloudInitNamespace}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get %s secrets in namespace %s: %w", cloudInitGetterSecret, CloudInitNamespace, err)
	}

	token := secret.Data["token"]
	if token == nil {
		return "", "", errors.New("failed to extract token from cloud-init secret")
	}

	apiServer, err := extractAPIServer(userdata)
	if err != nil {
		return "", "", fmt.Errorf("failed to extract api server address: %w", err)
	}

	return string(token), apiServer, nil
}

func CreateMachineCloudInitSecret(ctx context.Context, userdata, machineName string, client ctrlruntimeclient.Client) error {
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: CloudInitNamespace, Name: machineName}, secret); err != nil {
		if kerrors.IsNotFound(err) {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machineName,
					Namespace: CloudInitNamespace,
				},
				Data: map[string][]byte{"cloud_init": []byte(userdata)},
			}
			if err := client.Create(ctx, secret); err != nil {
				return fmt.Errorf("failed to create secret for userdata: %w", err)
			}
		}

		return fmt.Errorf("failed to fetch cloud-init secret: %w", err)
	}

	return nil
}

type file struct {
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"`
	Content     string `yaml:"content"`
}

func extractAPIServer(userdata string) (string, error) {
	files := &struct {
		WriteFiles []file `yaml:"write_files"`
	}{}

	if err := yaml.Unmarshal([]byte(userdata), files); err != nil {
		return "", fmt.Errorf("failed to unmarshal userdata: %w", err)
	}

	for _, file := range files.WriteFiles {
		if file.Path == "/etc/kubernetes/bootstrap-kubelet.conf" {
			config, err := clientcmd.RESTConfigFromKubeConfig([]byte(file.Content))
			if err != nil {
				return "", fmt.Errorf("failed to get kubeconfig from userdata: %w", err)
			}

			return config.Host, nil
		}
	}

	return "", errors.New("failed to find api-server host")
}
