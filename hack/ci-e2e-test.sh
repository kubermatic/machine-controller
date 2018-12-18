#!/usr/bin/env bash

set -euo pipefail

function cleanup {
    set +e

    # Clean up machines
    echo "Cleaning up machines."
    ./test/tools/integration/cleanup_machines.sh

    cd test/tools/integration
    for try in {1..20}; do
      # Clean up master
      echo "Cleaning up controller, attempt ${try}"
      # Clean up only the server, we want to keep the key as only one key may exist
      # for a given fingerprint
      terraform destroy -target=hcloud_server.machine-controller-test -force
      if [[ $? == 0 ]]; then break; fi
      echo "Sleeping for $try seconds"
      sleep ${try}s
    done
}
trap cleanup EXIT

# Install dependencies
echo "Installing dependencies."
apt update && apt install -y jq rsync unzip &&
curl --retry 5  -LO \
  https://storage.googleapis.com/kubernetes-release/release/v1.12.4/bin/linux/amd64/kubectl &&
chmod +x kubectl &&
mv kubectl /usr/local/bin

# Generate ssh keypair
echo "Set permissions for ssh key"
chmod 0700 $HOME/.ssh

# Initialize terraform
echo "Initalizing terraform"
cd test/tools/integration
make terraform
cp provider.tf{.disabled,}
terraform init --input=false --backend-config=key=$BUILD_ID
export TF_VAR_hcloud_token="${HZ_E2E_TOKEN}"
export TF_VAR_hcloud_sshkey_content="$(cat ~/.ssh/id_rsa.pub)"
export TF_VAR_hcloud_test_server_name="machine-controller-test-${BUILD_ID}"

for try in {1..20}; do
  set +e
  # Create environment at cloud provider
  echo "Creating environment at cloud provider."
  terraform import hcloud_ssh_key.default 265119
  terraform apply -auto-approve
  if [[ $? == 0 ]]; then break; fi
  echo "Sleeping for $try seconds"
  sleep ${try}s
done

set -e
cd -

# Build binaries
echo "Building machine-controller and webhook"
make machine-controller webhook

# Create kubeadm cluster and install machine-controller onto it
echo "Creating kubeadm cluster and install machine-controller onto it."
./test/tools/integration/provision_master.sh

# Run e2e test
echo "Running e2e test."
export KUBECONFIG=$GOPATH/src/github.com/kubermatic/machine-controller/.kubeconfig &&
go test -race -tags=e2e -parallel 240 -v -timeout 30m  ./test/e2e/... -identifier=$BUILD_ID
