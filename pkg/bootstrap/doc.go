/*
Copyright 2022 The Machine Controller Authors.

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

/*
package bootstrap contains the necessary type definitions to implement the external bootstrap
mechanism that machine-controller can use instead of generating instance user-data itself.

Any external bootstrap provider needs to implement the logic as laid out in this documentation.
This package can be imported to ensure the correct values and patterns are used.

machine-controller will expect a Secret object in the namespace defined by `CloudInitSettingsNamespace`,
using `CloudConfigSecretNamePattern` as a pattern to determine the Secret name. This secret must provide
valid user-data that will be passed to the cloud provider instance on creation.

Example code that determines the secret name for a specific Machine:

```
bootstrapSecretName := fmt.Sprintf(bootstrap.CloudConfigSecretNamePattern,
	referencedMachineDeployment,
	machine.Namespace,
	bootstrap.BootstrapCloudConfig)
```

*/

package bootstrap
