#!/usr/bin/env bash

set -e

make -C $(dirname $0)/.. machine-controller
$(dirname $0)/../machine-controller \
  -kubeconfig=$(dirname $0)/../.kubeconfig \
  -worker-count=50 \
  -logtostderr \
  -v=6 \
  -cluster-dns=10.10.10.10 \
  -internal-listen-address=0.0.0.0:8085
