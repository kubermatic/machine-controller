#!/usr/bin/env bash

set -euxo pipefail

BUILD_NUM=2

cd $(dirname $0)/kubevirt_dockerfiles

for flavor in ubuntu centos; do
	docker build \
		-t quay.io/kubermatic/machine-controller-kubevirt:$flavor-$BUILD_NUM \
		-f dockerfile.$flavor .
	docker push quay.io/kubermatic/machine-controller-kubevirt:$flavor-$BUILD_NUM
done
