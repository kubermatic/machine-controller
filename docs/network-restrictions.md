# Running behind a proxy

If nodes only have access via a HTTP proxy, you can let the machine-controller configure all new nodes to use this proxy.
For this the following flag must be set on the machine-controller side:
```bash
-node-http-proxy="http://192.168.1.1:3128"
```
This will set the following environment variables via /etc/environment on all nodes (lower & uppercase):
- `HTTP_PROXY`
- `HTTPS_PROXY`

`NO_PROXY` can be configured using a dedicated flag:
```bash
-node-no-proxy="10.0.0.1"
```

`-node-http-proxy` & `-node-no-proxy` must only contain IP addresses and/or domain names.

# Using a custom image registry

Except for custom workload, the kubelet requires access to the "pause" container.
This container is being used to keep the network namespace for each Pod alive.

By default the image `k8s.gcr.io/pause:3.1`* will be used.
If that image won't be accessible from the node, a custom image can be specified on the machine-controller:
```bash
-node-pause-image="192.168.1.1:5000/kubernetes/pause:3.1"
```

For ContainerLinux nodes the [hyperkube](https://github.com/kubernetes/kubernetes/tree/master/cluster/images/hyperkube) image must be accessible as well.
This is due to the usage of the [kubelet-wrapper](https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md).

By default the image `k8s.gcr.io/hyperkube-amd64` will be used.
If that image won't be accessible from the node, a custom image can be specified on the machine-controller:
```bash
# Do not set a tag. The tag depends on the used Kubernetes version of a machine.
# Example:
# A Node using v1.14.2 would use 192.168.1.1:5000/kubernetes/hyperkube-amd64:v1.14.2
-node-hyperkube-image="192.168.1.1:5000/kubernetes/hyperkube-amd64"
```

# Insecure registries

If nodes require access to insecure registries, all registries must be specified via a flag:
```bash
--node-insecure-registries="192.168.1.1:5000,10.0.0.1:5000"
```
