/*
Copyright 2020 The Machine Controller Authors.

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

package anexia

import (
	"fmt"
	"os"
	"time"

	"github.com/anexia/go-anxsdk"

	cloudproviderutil "k8c.io/machine-controller/pkg/cloudprovider/util"
	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
)

// getSDKClient creates a new go-anxsdk client. It is a variable so tests can
// point the provider at an httptest server; production code never reassigns it.
var getSDKClient = newSDKClient

// newSDKClient creates a new go-anxsdk client, reading the API token from the ANEXIA_TOKEN
// env var and reusing the same timeout/log-prefix behavior as the legacy client construction.
func newSDKClient(machineName *string) *anxsdk.Client {
	logPrefix := "[Anexia API]"
	if machineName != nil {
		logPrefix = fmt.Sprintf("[Anexia API for Machine %q]", *machineName)
	}

	httpClient := cloudproviderutil.HTTPClientConfig{
		Timeout:   120 * time.Second,
		LogPrefix: logPrefix,
	}.New()

	return anxsdk.NewClient(
		anxsdk.WithAPIKey(os.Getenv(anxtypes.AnxTokenEnv)),
		anxsdk.WithHTTPClient(&httpClient),
	)
}
