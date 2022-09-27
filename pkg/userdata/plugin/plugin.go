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

//
// Core UserData plugin.
//

// Package plugin provides the plugin side of the plugin mechanism.
// Individual plugins have to implement the provider interface,
// pass it to a new plugin instance, and call run.
package plugin

import (
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
)

// Provider defines the interface each plugin has to implement
// for the retrieval of the userdata based on the given arguments.
type Provider interface {
	UserData(req plugin.UserDataRequest) (string, error)
}
