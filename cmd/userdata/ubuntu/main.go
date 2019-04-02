//
// UserData plugin for Ubuntu.
//

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

func main() {
	// Parse flags.
	var debug bool

	flag.BoolVar(&debug, "debug", false, "Switch for enabling the plugin debugging")
	flag.Parse()

	// Instantiate provider and start plugin.
	var provider = &Provider{}
	var p = plugin.New(provider, debug)

	if err := p.Run(); err != nil {
		fmt.Printf("error running Ubuntu plugin: %v", err)
		os.Exit(-1)
	}
}
