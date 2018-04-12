#!/usr/bin/env bash
# vim: tw=500

set -euo pipefail

cd $(dirname $0)
export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')

ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }
cat <<EOF |ssh_exec
set -e
export AWS_E2E_TESTS_KEY_ID=$AWS_E2E_TESTS_KEY_ID
export AWS_E2E_TESTS_SECRET=$AWS_E2E_TESTS_SECRET
export DO_E2E_TESTS_TOKEN=$DO_E2E_TESTS_TOKEN

echo "Running E2E tests"
cd test/e2e
go test -tags=e2e -v -timeout 60m  ./... || (kubectl logs -n kube-system \$(kubectl get pods -n kube-system|egrep '^machine-con'|awk '{ print \$1 }'); exit 1)
EOF
