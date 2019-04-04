#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

cd $(dirname $0)/

export COREOS_IMAGE_NAME="machine-controller-e2e-coreos"
export UBUNTU_IMAGE_NAME="machine-controller-e2e-ubuntu"
export CENTOS_IMAGE_NAME="machine-controller-e2e-centos"

./setup-openstack-images.sh
