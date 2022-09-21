#!/usr/bin/env bash

# Copyright 2019 The Machine Controller Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail
set -x

cd "$(dirname "${BASH_SOURCE[0]}")"
MC_ROOT="$(cd ./../../.. && pwd -P)"

# We use variables prefixed with LC_* in order to be able to send them easily
# with SSH SendEnv, as SSH daemon this usually configured with:
# 'AcceptEnv LANG LC_*'.
export LC_DEPLOY_MACHINE="${1:-}"
export LC_ADDR=$(terraform output -json|jq '.ip.value' -r)
export LC_PRIV_ADDR=$(terraform output -json|jq '.private_ip.value' -r)
export LC_E2E_SSH_PUBKEY="${E2E_SSH_PUBKEY:-$(cat ~/.ssh/id_rsa.pub)}"
export LC_JOB_NAME="${JOB_NAME:-}"


ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${LC_ADDR} $@; }

for try in {1..100}; do
  if ssh_exec "systemctl stop apt-daily apt-daily-upgrade && systemctl mask apt-daily apt-daily-upgrade && exit"; then break; fi;
  sleep 1;
done

if [[ "${1:-deploy_machine_controller}"  != "do-not-deploy-machine-controller" ]]; then
rsync -avR  -e "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no" \
    ${MC_ROOT}/./{Makefile,examples/machine-controller.yaml,examples/webhook-certificate.cnf,machine-controller,machine-controller-userdata-*,Dockerfile,webhook} \
    root@${LC_ADDR}:/root/
fi

for try in {1..20}; do
  ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
    -o SendEnv=LC_ADDR \
    -o SendEnv=LC_DEPLOY_MACHINE \
    -o SendEnv=LC_E2E_SSH_PUBKEY \
    -o SendEnv=LC_JOB_NAME \
    root@${LC_ADDR} 'bash -s' < "${MC_ROOT}/test/tools/integration/master_install_script.sh" && break
  sleep ${try}s
done

for try in {1..20}; do
scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
  root@$LC_ADDR:/root/.kube/config \
  "${MC_ROOT}/.kubeconfig"
  if [[ $? == 0 ]]; then break; fi
  sleep ${try}s
done

# set up SSH port-forwarding if necessary
if [[ ! -z "${NUTANIX_E2E_PROXY_HOST:-}" ]]; then
  echo -n "${LC_PRIV_ADDR}" > ${MC_ROOT}/./priv_addr

  ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o ServerAliveInterval=5 -fNT -R ${LC_PRIV_ADDR}:${NUTANIX_E2E_PROXY_PORT}:${NUTANIX_E2E_PROXY_HOST}:${NUTANIX_E2E_PROXY_PORT} root@${LC_ADDR}
fi

