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

package plugins

import (
	"context"

	"go.uber.org/zap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PluginDriver manages the communications between the machine controller cloud provider and the bare metal env.
type PluginDriver interface {
	GetServer(context.Context) (Server, error)
	Validate(runtime.RawExtension) error
	ProvisionServer(context.Context, *zap.SugaredLogger, metav1.ObjectMeta, runtime.RawExtension, string) (Server, error)
	DeprovisionServer(context.Context) error
}

// Server represents the server/instance which exists in the bare metal env.
type Server interface {
	GetName() string
	GetID() string
	GetIPAddress() string
	GetMACAddress() string
	GetStatus() string
}
