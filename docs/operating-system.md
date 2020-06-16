# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | Container Linux | CentOS | RHEL | SLES |
|---|---|---|---|---|---|
| AWS | ✓ | ✓ | ✓ | ✓ | ✓ |
| Azure | ✓ | ✓ | ✓ | ✓ | x |
| Digitalocean  | ✓ | ✓ | ✓ | x | x |
| Google Cloud Platform | ✓ | ✓ | x | ✓ | x |
| Hetzner | ✓ | x | ✓ | x | x |
| Packet | ✓ | ✓ | ✓ | x | x |
| Openstack | ✓ | ✓ | ✓ | ✓ | x |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:
- `centos`
- `coreos`
- `rhel`
- `sles`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

### Supported OS versions

Note that the table below lists the OS versions that we are validating in our automated tests.
Machine controller may work with other OS versions that are not listed in the table but support won’t be provided.

|   | Versions |
|---|---|
| CentOS | 7.4.x, 7.6.x, 7.7.x |
| CoreOS | 1855.4.0, 2079.x.x, 2135.x.x, 2191.x.x, 2247.x.x, 2345.x.x |
| RHEL | 8.0, 8.1 |
| SLES |  SLES 15 SP1 |
| Ubuntu | 18.04 LTS |

### Ubuntu

```yaml
apiVersion: "cluster.k8s.io/v1alpha1"
kind: MachineDeployment
metadata:
  name: machine1
  namespace: kube-system
spec:
  paused: false
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  minReadySeconds: 0
  selector:
    matchLabels:
      foo: bar
  template:
    metadata:
      labels:
        foo: bar
    spec:
      providerConfig:
        value:
          ...
          operatingSystem: "ubuntu"
          operatingSystemSpec:
            # do a apt-get dist-upgrade on start and reboot if required
            distUpgradeOnBoot: true
```

### Container Linux

```yaml
apiVersion: "cluster.k8s.io/v1alpha1"
kind: MachineDeployment
metadata:
  name: machine1
  namespace: kube-system
spec:
  paused: false
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  minReadySeconds: 0
  selector:
    matchLabels:
      foo: bar
  template:
    metadata:
      labels:
        foo: bar
    spec:
      providerConfig:
        value:
          ...
          operatingSystem: "coreos"
          operatingSystemSpec:
            # disable auto update
            disableAutoUpdate: true
```

### CentOS

```yaml
apiVersion: "cluster.k8s.io/v1alpha1"
kind: MachineDeployment
metadata:
  name: machine1
  namespace: kube-system
spec:
  paused: false
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  minReadySeconds: 0
  selector:
    matchLabels:
      foo: bar
  template:
    metadata:
      labels:
        foo: bar
    spec:
      providerConfig:
        value:
          ...
          operatingSystem: "centos"
```
