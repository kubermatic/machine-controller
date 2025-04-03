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

package providerconfig

import (
	"time"
)

type ConfigVarResolver interface {
	GetDurationValue(configVar ConfigVarString) (time.Duration, error)
	GetDurationValueOrDefault(configVar ConfigVarString, defaultDuration time.Duration) (time.Duration, error)
	GetStringValue(configVar ConfigVarString) (string, error)
	GetStringValueOrEnv(configVar ConfigVarString, envVarName string) (string, error)
	GetBoolValue(configVar ConfigVarBool) (bool, bool, error)
	GetBoolValueOrEnv(configVar ConfigVarBool, envVarName string) (bool, error)
}
