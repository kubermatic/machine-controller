# Community Providers

`machine-controller` implements various cloud providers to create machines on. Some of these implementations have been
graciously provided by community members as external contributions. A huge thank you to all of them for improving `machine-controller`.

Since the core development team does not have the ability to automatically (or manually) test most of these providers
they are considered "community providers". The development team will keep them up to date to the best of their abilities
(e.g. when interfaces change and need to adjusted in the provider implementation), but cannot make any guarantees towards
bug fixes in a timely manner or their overall stability.

Because of that, `machine-controller` has a special flag to enable community providers called `--enable-community-providers`.
This flag needs to be configured both in the `machine-controller` and the `machine-controller-webhook` Deployments before
the providers listed below can be used.

## Provider List 

Currently, the following providers are considered "community providers":

- Linode
- Vultr
