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

package controller

import (
	"net/url"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const hostnamePlaceholder = "<MACHINE_NAME>"

func getOSMBootstrapUserdata(machineName string, bootstrapSecret corev1.Secret) string {
	bootstrapConfig := string(bootstrapSecret.Data["cloud-config"])

	// We have to inject the hostname i.e. machine name.
	bootstrapConfig = strings.ReplaceAll(bootstrapConfig, hostnamePlaceholder, machineName)
	// Data is HTML Encoded for ignition.
	bootstrapConfig = strings.ReplaceAll(bootstrapConfig, url.QueryEscape(hostnamePlaceholder), url.QueryEscape(machineName))
	return cleanupTemplateOutput(bootstrapConfig)
}

// cleanupTemplateOutput postprocesses the output of the template processing. Those
// may exist due to the working of template functions like those of the sprig package
// or template condition.
func cleanupTemplateOutput(output string) string {
	// Valid YAML files are not allowed to have empty lines containing spaces or tabs.
	// So far only cleanup.
	woBlankLines := regexp.MustCompile(`(?m)^[ \t]+$`).ReplaceAllString(output, "")
	return woBlankLines
}
