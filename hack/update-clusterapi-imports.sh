#!/usr/bin/env bash
# vim: tw=500

set -eux

cd $(dirname $0)/..

for clusterapi_sourcefile in pkg/machines/v1alpha1/machineset_types.go; do
	sed -i 's#sigs.k8s.io/cluster-api/pkg/apis/cluster/common#github.com/kubermatic/machine-controller/pkg/machines/common#' $clusterapi_sourcefile
done
