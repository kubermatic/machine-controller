# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | CentOS | Flatcar | RHEL | SLES | Amazon Linux 2 | Rocky Linux |
|---|---|---|---|---|---|---|---|
| AWS | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Azure | ✓ | ✓ | ✓ | ✓ | x | x | ✓ |
| Digitalocean  | ✓ | ✓ | x | x | x | x | ✓ |
| Google Cloud Platform | ✓ | x | x | x | x | x | x |
| Hetzner | ✓ | ✓ | x | x | x | x | ✓ |
| Equinix Metal | ✓ | ✓ | x | x | x | x | x |
| Openstack | ✓ | ✓ | ✓ | ✓ | x | x | ✓ |
| VMware Cloud Director | ✓ | x | x | x | x | x | x |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:

- `amzn2`
- `centos`
- `rhel`
- `rockylinux`
- `sles`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

### Supported OS versions

Note that the table below lists the OS versions that we are validating in our automated tests.
Machine controller may work with other OS versions that are not listed in the table but support won’t be provided.

|   | Versions |
|---|---|
| AmazonLinux2 | 2.x |
| CentOS | 7.4.x, 7.6.x, 7.7.x |
| RHEL | 8.0, 8.1 |
| Rocky Linux | 8.5 |
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
          operatingSystem: "flatcar"
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
