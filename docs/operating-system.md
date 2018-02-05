# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | Container Linux |
|---|---|---|
| AWS | ✓ | ✓ |
| Openstack | ✓ | ✓  |
| Digitalocean  | ✓ | ✓ |
| Hetzner | ✓ | x |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:
- `coreos`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

### Ubuntu

```yaml
apiVersion: "machine.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine1
spec:
  metadata:
    name: node1
  providerConfig:
    ...
    operatingSystem: "ubuntu"
    operatingSystemSpec:
      # do a apt-get dist-upgrade on start and reboot if required
      distUpgradeOnBoot: true
```

### Container Linux

```yaml
apiVersion: "machine.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine1
spec:
  metadata:
    name: node1
  providerConfig:
    ...
    operatingSystem: "coreos"
    operatingSystemSpec:
      # disable auto update
      disableAutoUpdate: true
```
