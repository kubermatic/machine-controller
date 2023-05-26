# How-to add a Provider

**Task of a provider implementation is building a bridge between the machine controller and a cloud API. This mainly means creating, configuring, retrieving, and deleting of machine instances. To do so a defined interface has to be implemented and integrated. Additionally the new provider has to provide an example manifest and finally integrated into the CI.**

## Implement the interface

### Interface description

The interface a cloud provider has to implement is located in the package `github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud`. It is named `Provider` and defines a small set of functions:

```go
AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error)
```

`AddDefaults` will read the `MachineSpec` and checks the defined values for needed fields. In case they are not set or invalid defaults can be applied.

```go
Validate(spec v1alpha1.MachineSpec) error
```

`Validate` validates the given machine's specification. In case of any error a _terminal error_ should be set. See `v1alpha1.MachineStatus` for more info.

```go
Get(machine *v1alpha1.Machine) (instance.Instance, error)
```

`Get` gets a node that is associated with the given machine. Note that this method can return a so called _terminal error_, which indicates that a manual interaction is required to recover from this state. See `v1alpha1.MachineStatus` for more info and `errors.TerminalError` type.

In case the instance cannot be found, the returned error has to be `github.com/kubermatic/machine-controller/pkg/cloudprovider/errors.ErrInstanceNotFound` for proper evaluation by the machine controller.

```go
GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error)
```

`GetCloudConfig` will return the cloud provider specific cloud-config, which gets consumed by the kubelet.

```go
Create(machine *v1alpha1.Machine, data *cloud.MachineCreateDeleteData, userdata string) (instance.Instance, error)
```

`Create` creates a cloud instance according to the given machine.

```go
Cleanup(machine *v1alpha1.Machine, data *cloud.MachineCreateDeleteData) (bool, error)
```

`Cleanup` will delete the instance associated with the machine and all associated resources. If all resources have been cleaned up, true will be returned. In case the cleanup involves asynchronous deletion of resources & those resources are not gone yet, false should be returned. This is to indicate that the cleanup is not done, but needs to be called again at a later point.

```go
MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error)
```

`MachineMetricsLabels` returns labels used for the _Prometheus_ metrics about created machines, e.g. instance type, instance size, region or whatever the provider deems interesting. Should always return a "size" label. This should not do any API calls to the cloud provider.

```go
MigrateUID(machine *v1alpha1.Machine, new types.UID) error
```

`MigrateUID` is called when the controller migrates types and the UID of the machine object changes. All cloud providers that use `Machine.UID` to uniquely identify resources must implement this.

```go
SetMetricsForMachines(machines v1alpha1.MachineList) error
```

`SetMetricsForMachines` allows providers to provide provider-specific metrics. This may be implemented as no-op.

### Implementation hints

Provider implementations are located in individual packages in `github.com/kubermatic/machine-controller/pkg/cloudprovider/provider`. Here see e.g. `hetzner` as a straight and good understandable implementation. Other implementations are there too, helping to understand the needed tasks inside and around the `Provider` interface implementation.

When retrieving the individual configuration from the provider specification a type for unmarshalling is needed. Here first the provider configuration is read and based on it the individual values of the configuration are retrieved. Typically the access data (token, ID/key combination, document with all information) alternatively can be passed via an environment variable. According
methods of the used `providerconfig.ConfigVarResolver` do support this.

For creation of new machines the support of the possible information has to be checked. The machine controller supports _CentOS_, _Flatcar_ and _Ubuntu_. In case one or more aren't supported by the cloud infrastructure the error `providerconfig.ErrOSNotSupported` has to be returned.

## Integrate provider into the Machine Controller

For each cloud provider a unique string constant has to be defined in file `types.go` in package `github.com/kubermatic/machine-controller/pkg/providerconfig`. Registration based on this constant is done in file `provider.go` in package `github.com/kubermatic/machine-controller/pkg/cloudprovider`.

## Add example manifest

For documentation of the different configuration options an according example manifest with helpful comments has to be added to `github.com/kubermatic/machine-controller/examples`. Naming scheme is `<package-name>-machinedeployment.yaml`.

## Integrate provider into CI

Like the example manifest a more concrete one named `machinedeployment-<package-name>.yaml` has to be added to `github.com/kubermatic/machine-controller/test/e2e/provisioning/testdata`. Additionally file `all_e2e_test.go` in package `github.com/kubermatic/machine-controller/test/e2e/provisioning` contains all provider tests. Like the existing ones the test for the new provider has to be placed here. Mainly it's the retrieval of test data, especially the access data, from the environment and the starting of the test scenarios.

Now the provider is ready to be added into the project for CI tests.

## References

- [Cloud Provider Interface](https://github.com/kubermatic/machine-controller/blob/main/pkg/cloudprovider/cloud/provider.go)
- [Implementation for Hetzner](https://github.com/kubermatic/machine-controller/blob/main/pkg/cloudprovider/provider/hetzner/provider.go)
- [Cloud Provider Type Definition](https://github.com/kubermatic/machine-controller/blob/main/pkg/providerconfig/types.go)
- [Registration of supported Cloud Providers](https://github.com/kubermatic/machine-controller/blob/main/pkg/cloudprovider/provider.go)
