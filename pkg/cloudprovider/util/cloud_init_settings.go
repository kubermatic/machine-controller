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
	"gopkg.in/yaml.v3"
	"html/template"
	"k8s.io/client-go/tools/clientcmd"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CloudInitNamespace       = "cloud-init-settings"
	jwtTokenNameSubsetPrefix = "cloud-init-getter-token"

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
      '{{ .ClusterHost }}/api/v1/namespaces/cloud-init-settings/secrets/{{ .SecretName }}'
    cat /etc/cloud/cloud.cfg.d/{{ .SecretName }} | jq '.data.cloud-init' -r | base64 -d > 99-bootstrap-config.cfg
    cloud-init clean
    cloud-init --file /etc/cloud/cloud.cfg.d/99-bootstrap-config.cfg init
      
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
`
)

func ExtractCloudInitSettingsToken(ctx context.Context, client ctrlruntimeclient.Client) (string, error) {
	secretList := corev1.SecretList{}
	if err := client.List(ctx, &secretList, &ctrlruntimeclient.ListOptions{Namespace: CloudInitNamespace}); err != nil {
		return "", fmt.Errorf("failed to list secrets in namespace %s: %v", CloudInitNamespace, err)
	}

	for _, secret := range secretList.Items {
		if strings.HasPrefix(secret.Name, jwtTokenNameSubsetPrefix) {
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
		Token       string
		SecretName  string
		ClusterHost string
	}{
		Token:       token,
		SecretName:  secretName,
		ClusterHost: clusterHost,
	}); err != nil {
		return "", fmt.Errorf("failed to execute bootstrap template: %v", err)
	}

	return buf.String(), nil
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
