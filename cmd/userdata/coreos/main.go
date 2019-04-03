//
// UserData plugin for CoreOS.
//

package main

import (
	"flag"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/userdata/convert"
	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	var provider = &Provider{}
	var p = plugin.New(convert.NewIgnition(provider), debug)

	if err := p.Run(); err != nil {
		glog.Fatalf("error running CoreOS plugin: %v", err)
	}
}
