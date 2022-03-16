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
set -o monitor

export TF_IN_AUTOMATION=true
export TF_CLI_ARGS="-no-color"

function cleanup {
  set +e

  # Clean up machines
  echo "Cleaning up machines."
  ./test/tools/integration/cleanup_machines.sh

  cd test/tools/integration
  for try in {1..20}; do
    # Clean up master
    echo "Cleaning up controller, attempt ${try}"
    terraform apply -destroy -auto-approve
    if [[ $? == 0 ]]; then break; fi
    echo "Sleeping for $try seconds"
    sleep ${try}s
  done

  # Kill background port forward if it's there
  pkill ssh || true
}
trap cleanup EXIT

# Install dependencies
echo "Installing dependencies..."
apt update && apt install -y jq rsync unzip genisoimage
curl --retry 5 --location --remote-name \
  https://storage.googleapis.com/kubernetes-release/release/v1.22.2/bin/linux/amd64/kubectl &&
  chmod +x kubectl &&
  mv kubectl /usr/local/bin

# Build binaries
echo "Building machine-controller and webhook..."
make download-gocache all

# Copy individual plugins with success control.
echo "Copying machine-controller plugins..."
cp machine-controller-userdata-* /usr/local/bin
ls -l /usr/local/bin

# Generate ssh key pair
echo "Generating SSH key pair..."
chmod 0700 $HOME/.ssh
ssh-keygen -t rsa -N "" -f ~/.ssh/id_ed25519

# Initialize terraform
echo "Initializing Terraform..."
cd test/tools/integration
make terraform
cp provider.tf{.disabled,}
terraform init --input=false --backend-config=key=$BUILD_ID
export TF_VAR_hcloud_token="${HZ_E2E_TOKEN}"
export TF_VAR_hcloud_sshkey_content="$(cat ~/.ssh/id_ed25519.pub)"
export TF_VAR_hcloud_sshkey_name="$BUILD_ID"
export TF_VAR_hcloud_test_server_name="machine-controller-test-${BUILD_ID}"

for try in {1..20}; do
  set +e
  # Create environment at cloud provider
  echo "Creating environment at cloud provider..."
  terraform apply -auto-approve
  TF_RC=$?
  if [[ $TF_RC == 0 ]]; then break; fi
  if [[ $TF_RC != 0 ]] && [[ $try -eq 20 ]]; then
    echo "Creating cloud provider env failed!"
    exit 1
  fi
  echo "Sleeping for $try seconds..."
  sleep ${try}s
done

set -e
cd -

echo "Creating kubeadm cluster and installing machine-controller into it..."
export E2E_SSH_PUBKEY="$(cat ~/.ssh/id_rsa.pub)"
./test/tools/integration/provision_master.sh

echo "Running e2e tests..."
if [[ ! -z "${NUTANIX_E2E_PROXY_HOST:-}" ]]; then
  vm_priv_addr=$(cat ./priv_addr)
  export NUTANIX_E2E_PROXY_URL="http://${NUTANIX_E2E_PROXY_USERNAME}:${NUTANIX_E2E_PROXY_PASSWORD}@${vm_priv_addr}:${NUTANIX_E2E_PROXY_PORT}/"
fi

export KUBECONFIG=$GOPATH/src/github.com/kubermatic/machine-controller/.kubeconfig
EXTRA_ARGS=""
if [[ $# -gt 0 ]]; then
  EXTRA_ARGS="-run $1"
fi
go test -race -tags=e2e -parallel 240 -v -timeout 70m ./test/e2e/... -identifier=$BUILD_ID $EXTRA_ARGS
