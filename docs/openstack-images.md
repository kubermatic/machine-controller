# Images

## Upload supported images to OpenStack

There is a script to upload all supported image to OpenStack.
```bash
./hack/setup-openstack-images.sh
```

By default all images will be named `machine-controller-${OS_NAME}`.
The image names can be overwritten using environment variables:
```bash
UBUNTU_IMAGE_NAME="ubuntu" ./hack/setup-openstack-images.sh
```
