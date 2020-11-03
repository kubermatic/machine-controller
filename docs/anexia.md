# Anexia Engine

This provider implementation is currently in **alpha** state.

## Supported Operating Systems

Only flatcar linux is currently supported and you explicitly have to set the provisioning mechanism to cloud-init by setting `machine.spec.providerSpec.value.operatingSystemSpec.provisioningUtility` to "cloud-init".

An example machine deployment can be found here: [examples/anexia-machinedeployment.yaml](../examples/anexia-machinedeployment.yaml)