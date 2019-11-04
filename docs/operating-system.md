# Operating system

## Support matrix

### Cloud provider

|   | Ubuntu | Container Linux | CentOS |
|---|---|---|---|
| AWS | ✓ | ✓ | ✓ |
| Openstack | ✓ | ✓ | ✓ |
| Digitalocean  | ✓ | ✓ | ✓ |
| Google Cloud Platform | ✓ | ✓ | x |
| Hetzner | ✓ | x | ✓ |
| Linode | ✓ | x | x |
| CherryServers | ✓ | x | ✓ |

## Configuring a operating system

The operating system to use can be set via `machine.spec.providerConfig.operatingSystem`.
Allowed values:
- `coreos`
- `ubuntu`

OS specific settings can be set via `machine.spec.providerConfig.operatingSystemSpec`.

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
