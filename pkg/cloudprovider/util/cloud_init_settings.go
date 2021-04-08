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
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CloudInitNamespace = "cloud-init-settings"
	jwtTokenNamePrefix = "cloud-init-getter-token"

	bootstrapCloudConfig = `#cloud-config

write_files:
- path: "/opt/bin/bootstrap"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    apt-get update
    apt-get install jq -y
    wget --no-check-certificate --quiet \
      --directory-prefix /etc/cloud/cloud.cfg.d/ \
      --method GET \
      --timeout=60 \
      --header 'Authorization: Bearer {{ .Token }}' \
      '{{ .ClusterHost }}/api/v1/namespaces/{{ .CloudInitSettingsNamespace }}/secrets/{{ .SecretName }}'
    cat /etc/cloud/cloud.cfg.d/{{ .SecretName }} | jq '.data.cloud_init' -r | base64 -d > /etc/cloud/cloud.cfg.d/99-provisioning-config.cfg
    rm /etc/cloud/cloud.cfg.d/{{ .SecretName }}
    cloud-init clean
    cloud-init --file /etc/cloud/cloud.cfg.d/99-provisioning-config.cfg init
    systemctl daemon-reload
    systemctl start setup.service 

- path: /etc/systemd/system/bootstrap.service
  permissions: "0644"
  content: |
    [Install]
    WantedBy=multi-user.target
    [Unit]
    Requires=network-online.target
    After=network-online.target
    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/opt/bin/bootstrap
runcmd:
- systemctl restart bootstrap.service`
)

func ExtractCloudInitSettingsToken(ctx context.Context, client ctrlruntimeclient.Client) (string, error) {
	secretList := corev1.SecretList{}
	if err := client.List(ctx, &secretList, &ctrlruntimeclient.ListOptions{Namespace: CloudInitNamespace}); err != nil {
		return "", fmt.Errorf("failed to list secrets in namespace %s: %v", CloudInitNamespace, err)
	}

	for _, secret := range secretList.Items {
		if strings.HasPrefix(secret.Name, jwtTokenNamePrefix) {
			if secret.Data != nil {
				jwtToken := secret.Data["token"]
				if jwtToken != nil {
					return string(jwtToken), nil
				}
			}
		}
	}

	return "", errors.New("failed to find cloud-init secret")
}

func GenerateCloudInitGetterScript(token, secretName, userdata string) (string, error) {
	tmpl, err := template.New("bootstrap").Parse(bootstrapCloudConfig)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	clusterHost, err := extractAPIServer(userdata)
	if err != nil {
		return "", fmt.Errorf("failed to extract api-server url: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, &struct {
		Token                      string
		SecretName                 string
		ClusterHost                string
		CloudInitSettingsNamespace string
	}{
		Token:                      token,
		SecretName:                 secretName,
		ClusterHost:                clusterHost,
		CloudInitSettingsNamespace: CloudInitNamespace,
	}); err != nil {
		return "", fmt.Errorf("failed to execute bootstrap template: %v", err)
	}

	return buf.String(), nil
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
				return fmt.Errorf("failed to create secret for userdata: %v", err)
			}
		}

		return fmt.Errorf("failed to fetch cloud-init secret: %v", err)
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
		return "", fmt.Errorf("failed to unmarshal userdata: %v", err)
	}

	for _, file := range files.WriteFiles {
		if file.Path == "/etc/kubernetes/bootstrap-kubelet.conf" {
			config, err := clientcmd.RESTConfigFromKubeConfig([]byte(file.Content))
			if err != nil {
				return "", fmt.Errorf("failed to get kubeconfig from userdata: %v", err)
			}

			return config.Host, nil
		}
	}

	return "", errors.New("failed to find api-server host")
}
