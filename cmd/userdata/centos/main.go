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
// UserData plugin for CentOS.
//

package main

import (
	"flag"
	"log"

	"go.uber.org/zap"

	machinecontrollerlog "github.com/kubermatic/machine-controller/pkg/log"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")

	logFlags := machinecontrollerlog.NewDefaultOptions()
	logFlags.AddFlags(flag.CommandLine)

	flag.Parse()

	if err := logFlags.Validate(); err != nil {
		log.Fatalf("Invalid options: %v", err)
	}

	rawLog := machinecontrollerlog.New(logFlags.Debug, logFlags.Format)
	log := rawLog.Sugar()

	// Instantiate provider and start plugin.
	var provider = &centos.Provider{}
	var p = userdataplugin.New(provider, debug)

	if err := p.Run(log); err != nil {
		log.Fatalw("Failed to run CentOS plugin", zap.Error(err))
	}
}
