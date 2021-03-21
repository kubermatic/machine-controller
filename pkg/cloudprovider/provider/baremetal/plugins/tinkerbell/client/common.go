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

// Package client contains a client wrapper for Tinkerbell.
package client

import (
	"errors"
)

// ErrNotFound is returned if a requested resource is not found.
var ErrNotFound = errors.New("resource not found")

// than parsing for these specific error message.
const (
	sqlErrorString    = "rpc error: code = Unknown desc = sql: no rows in result set"
	sqlErrorStringAlt = "rpc error: code = Unknown desc = SELECT: sql: no rows in result set"
)
