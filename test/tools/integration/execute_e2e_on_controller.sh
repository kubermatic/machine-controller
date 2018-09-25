#!/usr/bin/env bash

set -euo pipefail
cd $(dirname $0)

export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')

ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }

for VAR in AWS_E2E_TESTS_KEY_ID AWS_E2E_TESTS_SECRET \
  AZURE_E2E_TESTS_CLIENT_ID AZURE_E2E_TESTS_CLIENT_SECRET AZURE_E2E_TESTS_SUBSCRIPTION_ID AZURE_E2E_TESTS_TENANT_ID \
  DO_E2E_TESTS_TOKEN \
  E2E_SSH_PUBKEY \
  HZ_E2E_TOKEN OS_AUTH_URL \
  OS_AUTH_URL OS_DOMAIN OS_PASSWORD OS_REGION OS_USERNAME OS_TENANT_NAME OS_NETWORK_NAME \
  VSPHERE_E2E_ADDRESS VSPHERE_E2E_CLUSTER VSPHERE_E2E_PASSWORD VSPHERE_E2E_USERNAME \
  CIRCLE_BUILD_NUM; do
  echo "export ${VAR}=\"${!VAR}\"" >>/tmp/varfile
done

cd ../../..

echo "Building test binary"
export CGO_ENABLED=0
go test -c -tags=e2e -o /tmp/tests ./test/e2e/...

echo "Syncing data"
rsync -av  -e "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no" \
  ./test/e2e/provisioning/* /tmp/varfile /tmp/tests root@$ADDR:/root/


cat <<EOEXEC |ssh_exec
set -euo pipefail

cd /root
export KUBECONFIG=/etc/kubernetes/admin.conf
source varfile
echo "$E2E_SSH_PUBKEY" >> .ssh/authorized_keys

echo "starting tests"
./tests -test.parallel=240 -test.timeout=30m -test.v -identifier=$CIRCLE_BUILD_NUM

EOEXEC
