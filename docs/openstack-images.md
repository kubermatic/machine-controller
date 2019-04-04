# Images

## Upload supported images to OpenStack

There is a script to upload all supported image to OpenStack.
```bash
./hack/setup-openstack-images.sh
```

By default all images will be named `machine-controller-${OS_NAME}`.
The image names can be overwritten using environment variables:
```bash
COREOS_IMAGE_NAME="coreos" UBUNTU_IMAGE_NAME="ubuntu" CENTOS_IMAGE_NAME="centos" ./hack/setup-openstack-images.sh
```
