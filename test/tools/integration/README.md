# Integration testing

You can find some scripts here to do basic integration testing. Currently it
creates a single-node-cluster (servertype: `cx11`) via `kubeadm` at
[Hetzner cloud](https://www.hetzner.de/cloud) and verifies

* If the `machine-controller` pod successfully comes up

## Requirements

* Docker
* A [Hetzner Cloud account](https://www.hetzner.de/cloud)
* A SSH pubkey in `~/.ssh/id_rsa.pub`

## Usage

* `export HZ_TOKEN=<my_hetzner_cloud_token>`
* `make provision` to create and provision the environment
* Wait for the tests
* `make destroy`
