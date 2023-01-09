# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | CentOS | Flatcar | RHEL | Amazon Linux 2 | Rocky Linux |
|---|---|---|---|---|---|---|
| AWS | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Azure | ✓ | ✓ | ✓ | ✓ | x | ✓ |
| Digitalocean  | ✓ | ✓ | x | x | x | ✓ |
| Equinix Metal | ✓ | ✓ | ✓ | x | x | ✓ |
| Google Cloud Platform | ✓ | x | x | x | x | x |
| Hetzner | ✓ | ✓ | x | x | x | ✓ |
| KubeVirt | ✓ | ✓ | ✓ | ✓ | x | ✓ |
| Nutanix | ✓ | ✓ | x | x | x | x |
| Openstack | ✓ | ✓ | ✓ | ✓ | x | ✓ |
| VMware Cloud Director | ✓ | x | x | x | x | x |
| VSphere | ✓ | ✓ | ✓ | ✓ | x | ✓ |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:

- `amzn2`
- `centos`
- `flatcar`
- `rhel`
- `rockylinux`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

### Supported OS versions

Note that the table below lists the OS versions that we are validating in our automated tests.
Machine controller may work with other OS versions that are not listed in the table but support won’t be provided.

|   | Versions |
|---|---|
| AmazonLinux2 | 2.x |
| CentOS | 7.4.x, 7.6.x, 7.7.x |
| RHEL | 8.x |
| Rocky Linux | 8.5 |
| Ubuntu | 20.04 LTS, 22.04 LTS |
