//
// Userdata manager for plugin handling from machine controller side.
//

package manager

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
)

// Plugin manages the communication to one plugin. It is instantiated
// by the manager based on the directory scanning.
type Plugin struct {
	ctx  context.Context
	os   providerconfig.OperatingSystem
	port int
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via gRPC.
func newPlugin(ctx context.Context, os providerconfig.OperatingSystem, port int) (*Plugin, error) {
	p := &Plugin{
		ctx:  ctx,
		os:   os,
		port: port,
	}
	// Try starting the plugin.
	// TODO Add debug flag if wanted.
	plugin, err := findPlugin(pluginPrefix + string(p.os))
	if err != nil {
		return nil, err
	}
	argv := []string{"-listen-port", strconv.Itoa(p.port)}
	cmd := exec.CommandContext(p.ctx, plugin, argv...)
	// TODO stdout/stderr.
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return p, nil
}

// OperatingSystem returns the operating system this plugin is
// responsible for.
func (p *Plugin) OperatingSystem() providerconfig.OperatingSystem {
	return p.os
}

// UserData retrieves the user data of the given resource via
// plugin handling the communication.
func (p *Plugin) UserData(
	spec clusterv1alpha1.MachineSpec,
	kubeconfig *clientcmdapi.Config,
	ccProvider cloud.ConfigProvider,
	clusterDNSIPs []net.IP,
) (string, error) {
	return "", nil
}

// findPlugin searches for the plugin executable in machine controller
// directory, in working directory, and in path.
func findPlugin(filename string) (string, error) {
	// Create list to search in.
	ownDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dirs := []string{ownDir, workingDir}
	path := os.Getenv("PATH")
	pathDirs := strings.Split(path, string(os.PathListSeparator))
	dirs = append(dirs, pathDirs...)
	// Now take a look.
	for _, dir := range dirs {
		plugin := dir + string(os.PathSeparator) + filename
		_, err := os.Stat(plugin)
		if os.IsNotExist(err) {
			continue
		}
		return plugin, nil
	}
	return "", ErrPluginNotFound
}
