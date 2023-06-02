/*
Copyright 2022 The Machine Controller Authors.

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

package containerruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Opts struct {
	ContainerRuntime          string
	ContainerdVersion         string
	InsecureRegistries        string
	RegistryMirrors           string
	RegistryCredentialsSecret string
	PauseImage                string
	ContainerdRegistryMirrors RegistryMirrorsFlags
}

func BuildConfig(opts Opts) (Config, error) {
	var insecureRegistries []string
	for _, registry := range strings.Split(opts.InsecureRegistries, ",") {
		if trimmedRegistry := strings.TrimSpace(registry); trimmedRegistry != "" {
			insecureRegistries = append(insecureRegistries, trimmedRegistry)
		}
	}

	// we want to match e.g. docker.io=registry.docker-cn.com, having docker.io as the first
	// match group and registry.docker-cn.com as the second one.
	registryMirrorRegexp := regexp.MustCompile(`^([a-zA-Z0-9\.-]+)=(.*)`)

	if opts.ContainerdRegistryMirrors == nil {
		opts.ContainerdRegistryMirrors = make(RegistryMirrorsFlags)
	}

	for _, mirror := range strings.Split(opts.RegistryMirrors, ",") {
		if trimmedMirror := strings.TrimSpace(mirror); trimmedMirror != "" {
			registry := "docker.io"

			if matches := registryMirrorRegexp.FindStringSubmatch(trimmedMirror); matches != nil {
				registry = matches[1]
				trimmedMirror = matches[2]
			}

			if !strings.HasPrefix(trimmedMirror, "http") {
				trimmedMirror = "https://" + trimmedMirror
			}

			_, err := url.Parse(trimmedMirror)
			if err != nil {
				return Config{}, fmt.Errorf("incorrect mirror provided: %w", err)
			}

			if opts.ContainerdRegistryMirrors[registry] == nil {
				opts.ContainerdRegistryMirrors[registry] = make([]string, 0, 1)
			}

			opts.ContainerdRegistryMirrors[registry] = append(opts.ContainerdRegistryMirrors[registry], trimmedMirror)
		}
	}

	// Only validate registry credential here
	if opts.RegistryCredentialsSecret != "" {
		if secRef := strings.Split(opts.RegistryCredentialsSecret, "/"); len(secRef) != 2 {
			return Config{}, fmt.Errorf("-node-registry-credentials-secret is in incorrect format %q, should be in 'namespace/secretname'", opts.RegistryCredentialsSecret)
		}
	}

	return get(
		opts.ContainerRuntime,
		withInsecureRegistries(insecureRegistries),
		withRegistryMirrors(opts.ContainerdRegistryMirrors),
		withSandboxImage(opts.PauseImage),
		withContainerdVersion(opts.ContainerdVersion),
	), nil
}

func GetContainerdAuthConfig(ctx context.Context, client ctrlruntimeclient.Client, registryCredentialsSecret string) (map[string]AuthConfig, error) {
	registryCredentials := map[string]AuthConfig{}

	if secRef := strings.SplitN(registryCredentialsSecret, "/", 2); len(secRef) == 2 {
		var credsSecret corev1.Secret
		err := client.Get(ctx, types.NamespacedName{Namespace: secRef[0], Name: secRef[1]}, &credsSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve registry credentials secret object: %w", err)
		}

		switch credsSecret.Type {
		case corev1.SecretTypeDockerConfigJson:
			var regCred DockerCfgJSON
			if err := json.Unmarshal(credsSecret.Data[".dockerconfigjson"], &regCred); err != nil {
				return nil, fmt.Errorf("failed to unmarshal registry credentials: %w", err)
			}
			registryCredentials = regCred.Auths
		default:
			for registry, data := range credsSecret.Data {
				var regCred AuthConfig
				if err := json.Unmarshal(data, &regCred); err != nil {
					return nil, fmt.Errorf("failed to unmarshal registry credentials: %w", err)
				}
				registryCredentials[registry] = regCred
			}
		}
	}
	return registryCredentials, nil
}
