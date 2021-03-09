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
// UserData plugin for flatcar.
//

package main

import (
	"flag"

	"k8s.io/klog"

	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"
	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	var provider = &flatcar.Provider{}
	var p = userdataplugin.New(provider, debug)

	if err := p.Run(); err != nil {
		klog.Fatalf("error running flatcar plugin: %v", err)
	}
}
