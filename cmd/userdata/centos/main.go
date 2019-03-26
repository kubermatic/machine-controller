//
// UserData plugin for CentOS.
//

package main

import (
	"flag"
	"log"

	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var address string
	var debug bool

	flag.StringVar(&address, "address", "/tmp/machine-controller-userdata-centos.sock", "Unix domain socket for the CentOS UserData plugin")
	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	log.Printf("starting CentOS UserData plugin (address: %s / debug: %t)", address, debug)
	var provider = &Provider{}
	var p = plugin.New(provider, address, debug)

	if err := p.Start(); err != nil {
		log.Printf("CentOS UserData plugin ended: %v", err)
	}
}
