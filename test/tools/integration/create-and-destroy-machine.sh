#!/usr/bin/env bash

set -euo pipefail

cd $(dirname $0)
export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')

ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }
cat <<EOF |ssh_exec
set -e
echo "Testing create of a node via machine-controller...."
./verify \
  -input examples/machine-hetzner.yaml \
  -parameters "<< HETZNER_TOKEN >>=$HZ_TOKEN" \
  -logtostderr true || kubectl logs -n kube-system \$(kubectl get pods \
      -n kube-system|egrep '^machine-con'|awk '{ print \$1 }')
#./verify \
#  -input examples/machine-digitalocean.yaml \
#  -parameters "<< DIGITALOCEAN_TOKEN_BASE64_ENCODED >>=${DO_TOKEN:-undefined}" \
#  -logtostderr true
EOF
