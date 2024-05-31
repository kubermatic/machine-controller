# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | Flatcar |
|---|---|---|
| AWS | ✓ | ✓ |
| Openstack | ✓ | ✓ |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:

- `flatcar`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

### Supported OS versions

Note that the table below lists the OS versions that we are validating in our automated tests.
Machine controller may work with other OS versions that are not listed in the table but support won’t be provided.

|   | Versions |
|---|---|
| Ubuntu | 20.04 LTS, 22.04 LTS |
