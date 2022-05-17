/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vsphere

import (
	"context"
	"fmt"
	"net/url"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type Session struct {
	Client     *govmomi.Client
	Finder     *find.Finder
	Datacenter *object.Datacenter
}

type RESTSession struct {
	Client *rest.Client
}

// NewSession creates a vCenter client with initialized finder.
func NewSession(ctx context.Context, config *Config) (*Session, error) {
	vim25Client, err := createVim25Client(ctx, config)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	client := &govmomi.Client{
		Client:         vim25Client,
		SessionManager: session.NewManager(vim25Client),
	}

	if err = client.Login(ctx, url.UserPassword(config.Username, config.Password)); err != nil {
		return nil, fmt.Errorf("failed vsphere login: %w", err)
	}

	finder := find.NewFinder(client.Client, true)
	dc, err := finder.Datacenter(ctx, config.Datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to get vsphere datacenter: %w", err)
	}
	finder.SetDatacenter(dc)

	return &Session{
		Client:     client,
		Finder:     finder,
		Datacenter: dc,
	}, nil
}

// Logout closes the idling vCenter connections.
func (s *Session) Logout(ctx context.Context) {
	if err := s.Client.Logout(ctx); err != nil {
		utilruntime.HandleError(fmt.Errorf("vsphere client failed to logout: %w", err))
	}
}

func NewRESTSession(ctx context.Context, config *Config) (*RESTSession, error) {
	vim25Client, err := createVim25Client(ctx, config)
	if err != nil {
		return nil, err
	}

	client := rest.NewClient(vim25Client)

	if err = client.Login(ctx, url.UserPassword(config.Username, config.Password)); err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	return &RESTSession{
		Client: client,
	}, nil
}

// Logout closes the idling vCenter connections.
func (s *RESTSession) Logout(ctx context.Context) {
	if err := s.Client.Logout(ctx); err != nil {
		utilruntime.HandleError(fmt.Errorf("vsphere REST client failed to logout: %w", err))
	}
}

func createVim25Client(ctx context.Context, config *Config) (*vim25.Client, error) {
	clientURL, err := url.Parse(fmt.Sprintf("%s/sdk", config.VSphereURL))
	if err != nil {
		return nil, err
	}

	// creating the govmoni Client in roundabout way because we need to set the proper CA bundle: reference https://github.com/vmware/govmomi/issues/1200
	soapClient := soap.NewClient(clientURL, config.AllowInsecure)
	// set our CA bundle
	soapClient.DefaultTransport().TLSClientConfig.RootCAs = util.CABundle

	return vim25.NewClient(ctx, soapClient)
}
