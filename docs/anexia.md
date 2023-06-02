# Anexia Engine

This provider implementation is currently in **alpha** state.

## Supported Operating Systems

Only flatcar linux is currently supported and you explicitly have to set the provisioning mechanism to cloud-init by setting `machine.spec.providerSpec.value.operatingSystemSpec.provisioningUtility` to "cloud-init".

An example machine deployment can be found here: [examples/anexia-machinedeployment.yaml](../examples/anexia-machinedeployment.yaml)

## Templates

You can configure the template to use by its name (using the attribute `template`) or its identifier (using the attribute `templateID`).

When specifying the template by its name, the template build to use can optionally be set (attribute `templateBuild`). Omitting `templateBuild` will yield the latest available build (at time the time of creating the `Machine`) for the specified named template.

Template identifiers (attribute `templateID`) always link to a given `template`-`templateBuild` combination, so using the identifier in configuration has the same drawback as specifying an exact build to use.

Templates are rotated pretty often to include security patches and other updates. Outdated versions of templates are not retained and get removed after some time. Because of this, we do not recommend using the `templateID` attribute or pinning to a fixed build unless really required.

To retrieve all available templates against a given location:

```
https://engine.anexia-it.com/api/vsphere/v1/provisioning/templates.json/<location identifier>/templates?page=1&limit=50&api_key=<API Key>
```
