# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | Container Linux | CentOS | RHEL | SLES |
|---|---|---|---|---|---|
| Alibaba Cloud | ? | x | ?  | x | x |
| AWS | ✓ | ✓ | ✓ | ? | ✓ |
| Azure | ✓ | x | x | x | x |
| Digitalocean  | ✓ | ✓ | ✓ | ? | ? |
| Google Cloud Platform | ✓ | ✓ | x | ? | ? |
| Hetzner | ✓ | x | ✓ | x | ✓ |
| Linode | ✓ | x | x | ? | ? |
| Packet | ✓ | ✓ | ✓ | ? | ? |
| Openstack | ✓ | ✓ | ✓ | ? | ? |

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
| CentOS | 7.6.x, 7.7.x |
| CoreOS | 1855.4.0, 2079.3.0, 2345.3.0 |
| RHEL | 8.x |
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
