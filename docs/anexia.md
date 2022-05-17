# Anexia Engine

This provider implementation is currently in **alpha** state.

## Supported Operating Systems

Only flatcar linux is currently supported and you explicitly have to set the provisioning mechanism to cloud-init by setting `machine.spec.providerSpec.value.operatingSystemSpec.provisioningUtility` to "cloud-init".

An example machine deployment can be found here: [examples/anexia-machinedeployment.yaml](../examples/anexia-machinedeployment.yaml)

## Templates

To retrieve all available templates against a given location:

```
https://engine.anexia-it.com/api/vsphere/v1/provisioning/templates.json/<location identifier>/templates?page=1&limit=50&api_key=<API Key>
```

Templates are rotated pretty often, to include updates and latest security patches. Outdated versions of templates are not retained as a result and they get removed after some time.
