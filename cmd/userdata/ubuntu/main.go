//
// UserData plugin for Ubuntu.
//

package main

import (
	"flag"

	"github.com/golang/glog"

	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"
)

func main() {
	// Parse flags.
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	var provider = &ubuntu.Provider{}
	var p = userdataplugin.New(provider, debug)

	if err := p.Run(); err != nil {
		glog.Fatalf("error running Ubuntu plugin: %v", err)
	}
}
