//
// UserData plugin for CoreOS.
//

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	req := os.Getenv(plugin.EnvUserDataRequest)
	ioutil.WriteFile("/tmp/machine-controller-userdata-coreos.log", []byte(req), 0644)

	// Instantiate provider and start plugin.
	var provider = &Provider{}
	var p = plugin.New(provider, debug)

	if err := p.Run(); err != nil {
		fmt.Printf("error running CoreOS plugin: %v", err)
		os.Exit(-1)
	}
}
