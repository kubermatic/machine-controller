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
	"fmt"
	"sort"
	"strings"
)

type RegistryMirrorsFlags map[string][]string

func (fl RegistryMirrorsFlags) Set(val string) error {
	split := strings.SplitN(val, "=", 2)
	if len(split) != 2 {
		return fmt.Errorf("should have exactly 1 =")
	}

	key, value := split[0], split[1]
	slice := fl[key]
	slice = append(slice, value)
	fl[key] = slice

	return nil
}

func (fl RegistryMirrorsFlags) String() string {
	var (
		registryNames []string
		result        []string
	)

	for registryName := range fl {
		registryNames = append(registryNames, registryName)
	}

	sort.Strings(registryNames)

	for _, registryName := range registryNames {
		for _, mirror := range fl[registryName] {
			result = append(result, fmt.Sprintf("%s=%s", registryName, mirror))
		}
	}

	return fmt.Sprintf("%v", result)
}
