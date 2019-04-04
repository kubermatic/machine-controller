//
// UserData plugin for CentOS.
//

package main

import (
	"flag"

	"github.com/golang/glog"

	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	var provider = &Provider{}
	var p = userdataplugin.New(provider, debug)

	if err := p.Run(); err != nil {
		glog.Fatalf("error running CentOS plugin: %v", err)
	}
}
