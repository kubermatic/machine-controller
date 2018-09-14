#!/usr/bin/env bash

set -euo pipefail

cd $(dirname $0)/..

const_val=$(grep 'TypeRevisionCurrentVersion = "' \
  pkg/apis/cluster/v1alpha1/conversions/conversions.go|awk '{print $3 }'|tr -d '"')

constraint_val=$(grep sigs.k8s.io/cluster-api -A1 Gopkg.toml \
  |grep revision|awk '{ print $3 }'|tr -d '"')

if [[ "$const_val" != "$constraint_val" ]]; then
  echo "Error! TypeRevisionCurrentVersion constant in pkg/apis/cluster/v1alpha1/conversions/conversions.go does not match the constraint for sigs.k8s.io/cluster-api in Gopkg.toml!"
  exit 1
fi
