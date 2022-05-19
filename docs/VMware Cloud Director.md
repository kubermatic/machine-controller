# VMware Cloud Director

## Prerequisites

The following things should be configured before managing machines on VMware Cloud Director:

- Dedicated Organization VDC has been created.
- Required catalog and templates for creating VMs have been added to the organization VDC.
- VApp has been created that will be used to encapsulate all the VMs.
- Direct, routed or isolated network has been created. And the virtual machines within the vApp can communicate over that network.

## Configuration Options

An example `MachineDeployment` can be found [here](../examples/vmware-cloud-director-machinedeployment.yaml).
