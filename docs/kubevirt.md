# Kubevirt

In order to use the machine-controller to create machines using [Kubevirt](https://kubevirt.io)
you must first install the latter. We provider a manifest for this, simply run `kubectl apply -f examples/kubevirt-operator-0.17.4.yaml`.

Afterwards, you can use the provided `exampes/examples/kubevirt-machinedeployment.yaml` as base. There
are some things you need to keep in mind:

* The machine-controller will create `VMIs` that have the same name as the underlying `machine`. To
avoid collisions, use one namespace per cluster that runs the `machine-controller`
* Service CIDR range: The CIDR ranges of the cluster that runs Kubevirt and the cluster that hosts the machine-controller must not overlap, otherwise routing of services that run in the kubevirt cluster
 wont work anymore. THis is especially important for the DNS ClusterIP.
* In order to create VirtualMachineInstances deployed with CoreOS, you should use `coreos_production_qemu_image.img.bz2`, which can be found in the stable releases
[here](https://stable.release.core-os.net/amd64-usr/).