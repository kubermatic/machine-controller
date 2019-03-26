//
// UserData plugin manager.
//

package manager

import (
	"context"
	"errors"
	"net"
	"net/rpc"
	"os/exec"
	"path/filepath"
	"syscall"

	"k8s.io/apimachinery/pkg/util/wait"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

const (
	// Interval and timeout for plugin connection polling.
	pollInterval = 20 * time.Millisecond
	pollTimeout  = 5 * time.Second

	// pluginPrefix has to be the prefix of all plugin filenames.
	pluginPrefix = "machine-controller-userdata-"
)

// Plugin manages the communication to one plugin. It is instantiated
// by the manager based on the directory scanning.
type Plugin struct {
	ctx    context.Context
	os     providerconfig.OperatingSystem
	debug  bool
	cancel func()
	client *rpc.Client
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via net/rpc.
func newPlugin(ctx context.Context, os providerconfig.OperatingSystem, debug bool) (*Plugin, error) {
	pluginCtx, cancel := context.WithCancel(ctx)
	p := &Plugin{
		ctx:    pluginCtx,
		os:     os,
		debug:  debug,
		cancel: cancel,
	}
	if err = p.startPlugin(); err != nil {
		return nil, err
	}
	return p, nil
}

// Stop terminates the plugin by closing the client and cancel the
// plugin context.
func (p *Plugin) Stop() error {
	defer p.cancel()
	return p.client.Close()
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
	req := &plugin.UserDataRequest{
		MachineSpec: spec,
		KubeConfig:  kubeconfig,
		CloudConfig: ccProvider,
		DNSIPs:      cluserDNSIPs,
	}
	var repl plugin.UserDataResponse
	err = client.Call("Plugin.UserData", req, &repl)
	if err != nil {
		return "", err
	}
	if resp.Err != "" {
		return "", errors.New(resp.Err)
	}
	return resp.UserData, nil
}

// startPlugin tries to find the find the according file
// and start it as child process of the machine controlle.
func (p *Plugin) startPlugin() error {
	name := pluginPrefix + string(p.os)
	plugin, err := findPlugin(name)
	if err != nil {
		return err
	}
	address = "/tmp/" + name + ".sock"
	// Delete probabely remaining socket file, error can be ignored.
	os.Remove(address)
	// Start the plugin.
	argv := []string{"-address", address}
	if p.debug {
		argv = append(argv, "-debug")
	}
	cmd := exec.CommandContext(p.ctx, plugin, argv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	} else {
		cmd.SysProcAttr.Setpgid = true
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Wait to connect the fresh started plugin.
	return wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		client, err := rpc.DialHTTPPath("unix", p.address, plugin.RPCPath)
		if err != nil {
			p.client = client
			return true, nil
		}
		// Not yet done.
		return false, nil
	})
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
