package vsphere

import (
	"context"
	"fmt"
	"net/url"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type Session struct {
	Client     *govmomi.Client
	Finder     *find.Finder
	Datacenter *object.Datacenter
}

// NewSession creates a vCenter client with initialized finder
func NewSession(ctx context.Context, config *Config) (*Session, error) {
	clientURL, err := url.Parse(fmt.Sprintf("%s/sdk", config.VSphereURL))
	if err != nil {
		return nil, err
	}
	clientURL.User = url.UserPassword(config.Username, config.Password)

	client, err := govmomi.NewClient(ctx, clientURL, config.AllowInsecure)
	if err != nil {
		return nil, fmt.Errorf("failed to build client: %v", err)
	}

	finder := find.NewFinder(client.Client, true)
	dc, err := finder.Datacenter(ctx, config.Datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to get vsphere datacenter: %v", err)
	}
	finder.SetDatacenter(dc)

	return &Session{
		Client:     client,
		Finder:     finder,
		Datacenter: dc,
	}, nil
}

// Logout closes the idling vCenter connections
func (s *Session) Logout() {
	if err := s.Client.Logout(context.Background()); err != nil {
		utilruntime.HandleError(fmt.Errorf("vsphere client failed to logout: %s", err))
	}
}
