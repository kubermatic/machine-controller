# Nutanix Prism Central

Currently the `machine-controller` implementation of Nutanix supports the [Prism v3 API](https://www.nutanix.dev/reference/prism_central/v3/) to create `Machines`.

## Prerequisites

The `nutanix` provider assumes several things to be preexisting. You need:

- Credentials and access information for a Nutanix Prism Central instance (endpoint, port, username and password).
- The name of a Nutanix cluster to create the VMs for Machines on.
- The name of a subnet on the given Nutanix cluster that the VMs' network interfaces will be created on.
- An image name that will be used to create the VM for (must match the configured operating system).
- **Optional**: The name of a project that the given credentials have access to, to create the VMs in. If none is provided, the VMs are created without a project.

## Configuration Options

An example `MachineDeployment` can be found [here](../examples/nutanix-machinedeployment.yaml).
