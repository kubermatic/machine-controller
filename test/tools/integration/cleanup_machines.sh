#!/usr/bin/env bash

set -euo pipefail
set -x

cd $(dirname $0)

export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')


ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }


cat <<EOEXEC |ssh_exec
set -ex
kubectl delete machine --all
for try in {1..10}; do
  if kubectl get machine 2>&1|grep -q  'No resources found.'; then exit 0; fi
  sleep 10s
done

echo "Error: couldn't delete all machines!"
exit 1
EOEXEC
