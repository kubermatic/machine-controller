package anexia

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	v1 "k8s.io/api/core/v1"

	"github.com/anexia-it/go-anxcloud/pkg/client"
	"github.com/anexia-it/go-anxcloud/pkg/info"
	"github.com/anexia-it/go-anxcloud/pkg/powercontrol"
	"github.com/anexia-it/go-anxcloud/pkg/provisioning/ips"
	"github.com/anexia-it/go-anxcloud/pkg/provisioning/progress"
	"github.com/anexia-it/go-anxcloud/pkg/provisioning/vm"
	"github.com/anexia-it/go-anxcloud/pkg/search"
)

type anexiaServer struct {
	server     *search.VM
	powerState powercontrol.State
	info       *info.Info
}

func GetFromAnexia(ctx context.Context, name string, client client.Client) (*anexiaServer, error) {
	searchResult, err := search.ByName(ctx, fmt.Sprintf("%%-%s", name), client)
	if err != nil {
		return nil, err
	}

	if len(searchResult) != 1 {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	vm := &searchResult[0]

	powerState, err := powercontrol.Get(ctx, vm.Identifier, client)
	if err != nil {
		return nil, fmt.Errorf("could not get machine powerstate, due to: %w", err)
	}

	info, err := info.Get(ctx, vm.Identifier, client)
	if err != nil {
		return nil, fmt.Errorf("could not get machine info, due to: %w", err)
	}

	return &anexiaServer{
		server:     &searchResult[0],
		powerState: powerState,
		info:       &info,
	}, nil
}

func CreateAnexiaVM(ctx context.Context, config *Config, name, userdata string, client client.Client) (*anexiaServer, error) {
	ips, err := ips.GetFree(ctx, config.LocationID, config.VlanID, client)
	if err != nil {
		return nil, fmt.Errorf("getting free ips failed: %w", err)
	}
	if len(ips) < 1 {
		return nil, fmt.Errorf("no free ip available: %w", err)
	}

	networkInterfaces := []vm.Network{{
		NICType: "vmxnet3",
		IPs:     []string{ips[0].Identifier},
		VLAN:    config.VlanID,
	}}

	templateType := "templates"

	definition := vm.NewDefinition(
		config.LocationID,
		templateType,
		config.TemplateID,
		name,
		config.Cpus,
		config.Memory,
		config.DiskSize,
		networkInterfaces,
	)

	encodedUserdata := base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf(
			"anexia: true\n\n#cloud-config\n%s",
			userdata,
		)),
	)
	definition.Script = encodedUserdata

	sshkey, err := ssh.NewKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ssh key: %v", err)
	}

	definition.SSH = sshkey.PublicKey

	provisionResponse, err := vm.Provision(ctx, definition, client)
	if err != nil {
		return nil, fmt.Errorf("provisioning vm failed: %w", err)
	}

	_, err = progress.AwaitCompletion(ctx, provisionResponse.Identifier, client)
	if err != nil {
		return nil, fmt.Errorf("waiting for VM provisioning failed: %w", err)
	}

	// Sleep to work around a race condition in the anexia API
	time.Sleep(time.Second * 5)

	return GetFromAnexia(ctx, name, client)
}

func Remove(ctx context.Context, name string, client client.Client) error {
	instance, err := GetFromAnexia(ctx, name, client)
	if err != nil {
		return err
	}

	return vm.Deprovision(ctx, instance.server.Identifier, false, client)
}

func (s *anexiaServer) Name() string {
	if s.server == nil {
		return "none"
	}

	return s.server.Name
}

func (s *anexiaServer) ID() string {
	if s.server == nil {
		return "none"
	}

	return s.server.Identifier
}

func (s *anexiaServer) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}

	if s.server == nil {
		return addresses
	}

	for _, network := range s.info.Network {
		fmt.Printf("network: %+v\n", network)
		for _, ip := range network.IPv4 {
			addresses[ip] = v1.NodeExternalIP
		}
		for _, ip := range network.IPv6 {
			addresses[ip] = v1.NodeExternalIP
		}

		// TODO mark RFC1918 and RFC4193 addresses as internal
	}

	return addresses
}

func (s *anexiaServer) Status() instance.Status {
	if s.info != nil {
		if s.info.Status == "poweredOn" {
			return instance.StatusRunning
		}
	}

	return instance.StatusUnknown
}
