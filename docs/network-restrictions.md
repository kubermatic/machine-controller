# Running behind a proxy

If nodes only have access via a HTTP proxy, you can let the machine-controller configure all new nodes to use this proxy.
For this the following flag must be set on the machine-controller side:
```bash
-node-http-proxy="http://192.168.1.1:3128"
```
This will set the following environment variables via /etc/environment on all nodes (lower & uppercase):
- HTTP_PROXY
- HTTPS_PROXY
- NO_PROXY (Will always be set to `localhost,127.0.0.1`)

# Using a custom image registry

If docker/OCI images can only be accessed using a dedicated registry, you can let the machine-controller configure all new nodes to use a own registry.
For this the following flag must be set on the machine-controller side:
```bash
-node-image-registry="192.168.1.1:5000"
```
This will instruct the nodes to only pull images, which are required by the Kubelet and the OS, from this repository.

The following images are currently being required by the Kubelet & OS:
- `k8s.gcr.io/pause:3.1`
  When a custom registry like `192.168.1.1:5000` is specified the full image name will be: `192.168.1.1/machine-controller/pause:3.1`
- `k8s.gcr.io/hyperkube-amd64:v1.10.3` (The tag matches the used Kubernetes version)
  When a custom registry like `192.168.1.1:5000` is specified the full image name will be: `192.168.1.1/machine-controller/hyperkube-amd64:v1.12.0`

## Retag images

A simple helper script to push the images can be found in the hack folder: [../hack/retag-images.sh](../hack/retag-images.sh)
```bash
# Pulls, retags & pushes the above mentioned images
./hack/retag-images.sh my-awesome-registry:5000 v1.14.2
```
