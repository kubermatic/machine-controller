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


## Kubelet images

### CoreOS ContainerLinux
For ContainerLinux nodes, the [hyperkube][1] image must be accessible as well. This is due to the usage of the
[kubelet-wrapper][2].

By default the image `k8s.gcr.io/hyperkube-amd64` will be used. If that image won't be accessible from the node, a
custom image can be specified on the machine-controller:
```bash
# Do not set a tag. The tag depends on the used Kubernetes version of a machine.
# Example:
# A Node using v1.14.2 would use 192.168.1.1:5000/kubernetes/hyperkube-amd64:v1.14.2
-node-hyperkube-image="192.168.1.1:5000/kubernetes/hyperkube-amd64"
```

### Flatcar Linux
For Flatcar Linux nodes, the [hyperkube][1] or [kubelet][3] image must be accessible as well. This is due to the fact
that kubelet is running as a docker container. For kubelet version `< 1.18` hyperkube will be used, otherwise `kubelet`
image.

By default the image `quay.io/poseidon/kubelet` will be used. If that image won't be accessible from the node, a custom
image can be specified on the machine-controller:
```bash
# Do not set a tag. The tag depends on the used Kubernetes version of a machine.
# Example:
# A Node using v1.14.2 would use 192.168.1.1:5000/kubernetes/hyperkube-amd64:v1.14.2
-node-hyperkube-image="192.168.1.1:5000/kubernetes/hyperkube-amd64"
-node-kubelet-image="192.168.1.1:5000/my-custom/kubelet-amd64"
```

# Insecure registries

If nodes require access to insecure registries, all registries must be specified via a flag:
```bash
--node-insecure-registries="192.168.1.1:5000,10.0.0.1:5000"
```

[1]: https://console.cloud.google.com/gcr/images/google-containers/GLOBAL/hyperkube
[2]: https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md
[3]: https://quay.io/poseidon/kubelet
