#!/usr/bin/env bash
# vim: tw=500

set -euo pipefail

cd $(dirname $0)
export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')

export OS=${1:-ubuntu}
export TYPE=${2:-hetzner}

ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }
cat <<EOF |ssh_exec
set -e
echo "Testing create of a node via machine-controller...."
./verify \
  -input examples/machine-$TYPE.yaml \
  -parameters "machine1=machine-${TYPE}-${OS}" \
  -parameters "<< HETZNER_TOKEN >>=$HZ_TOKEN,cri-o=docker,node1=testnode-${CIRCLE_BUILD_NUM:-local},ubuntu=${OS}" \
  -parameters "<< VSPHERE_PASSWORD >>=${VSPHERE_PASSWORD:-undef},<< VSPHERE_USERNAME >>=${VSPHERE_USERNAME:-undef}" \
  -parameters "<< VSPHERE_ADDRESS >>=${VSPHERE_ADDRESS:-undef}" \
  -logtostderr true || (kubectl logs -n kube-system \$(kubectl get pods \
      -n kube-system|egrep '^machine-con'|awk '{ print \$1 }'); exit 1)
EOF
