/*
Copyright 2021 The Machine Controller Authors.
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

package ssh

import (
	"context"
	"sync"
	"time"

	"errors"
)

// Connector holds a map of Connections
type Connector struct {
	lock        sync.Mutex
	connections map[int]Connection
	ctx         context.Context
}

// NewConnector constructor
func NewConnector(ctx context.Context) *Connector {
	return &Connector{
		connections: make(map[int]Connection),
		ctx:         ctx,
	}
}

// Tunnel returns established SSH tunnel
func (c *Connector) Tunnel(host HostConfig) (Tunneler, error) {
	conn, err := c.Connect(host)
	if err != nil {
		return nil, err
	}

	tunn, ok := conn.(Tunneler)
	if !ok {
		err = errors.New("unable to assert Tunneler")
	}

	return tunn, err
}

// Connect to the node
func (c *Connector) Connect(host HostConfig) (Connection, error) {
	var err error

	c.lock.Lock()
	defer c.lock.Unlock()

	conn, found := c.connections[host.ID]
	if !found {
		opts := sshOpts(host)
		opts.Context = c.ctx
		conn, err = NewConnection(c, opts)
		if err != nil {
			return nil, err
		}

		c.connections[host.ID] = conn
	}

	return conn, nil
}

func (c *Connector) forgetConnection(conn *connection) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for k := range c.connections {
		if c.connections[k] == conn {
			delete(c.connections, k)
		}
	}
}

func sshOpts(host HostConfig) Opts {
	var hostname = host.PublicAddress
	if host.Hostname != "" {
		hostname = host.Hostname
	}

	//var (
	//	privateKeyPath = host.SSHPrivateKeyFile
	//	privateKey     string
	//)
	//
	//if privateKey = os.Getenv("SSH_PRIVATE_KEY_RAW"); privateKey != "" {
	//	// if the private key content is available then no need to check for private key files
	//	privateKeyPath = ""
	//}

	return Opts{
		Username: host.SSHUsername,
		Port:     host.SSHPort,
		Hostname: hostname,
		KeyFile:  host.SSHPrivateKeyFile,
		Timeout:  10 * time.Second,
		//PrivateKey: privateKey,
	}
}
