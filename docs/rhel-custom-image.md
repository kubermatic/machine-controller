# RedHat Enterprise Linux

Cloud providers which are listed below, support using RHEL as an operating system option:

- AWS
- Openstack

####  AWS
First of all the RHEL gold image AMIs have to be enabled from the [RedHat Customer Portal](https://access.redhat.com/public-cloud/aws) (this requires a [cloud-provider subscription](https://access.redhat.com/public-cloud)).

Afterwards, new images will be added to the aws account under EC2-> Images-> AMIs-> Private Images.
Once the images are available in the aws account, the image id for RHEL (supported versions are mentioned [here](./operating-system.md#supported-os-versions)) should be then added to the `MachineDeployment` spec to the field `ami`.

####  Openstack
Once RHEL images(e.g: Red Hat Enterprise Linux 8.x KVM Guest Image) is uploaded to openstack, the image name should be used in
the `MachineDeployment` field `.spec.template.spec.providerSpec.value.cloudProviderSpec.image`.
