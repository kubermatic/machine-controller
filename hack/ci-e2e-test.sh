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
      terraform destroy -force
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

# Build binaries
echo "Building machine-controller and webhook"
make machine-controller webhook

# Copy individual plugins with success control.
cp machine-controller-userdata-centos /usr/local/bin
cp machine-controller-userdata-coreos /usr/local/bin
cp machine-controller-userdata-ubuntu /usr/local/bin
ls -l /usr/local/bin

# Generate ssh keypair
echo "Generating ssh keypair"
chmod 0700 $HOME/.ssh
ssh-keygen -t rsa -N ""  -f ~/.ssh/id_ed25519

# Initialize terraform
echo "Initalizing terraform"
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
  echo "Creating environment at cloud provider."
  terraform apply -auto-approve
  TF_RC=$?
  if [[ $TF_RC == 0 ]]; then break; fi
  if [[ $TF_RC != 0 ]] && [[ $try -eq 20 ]]; then
    echo "Creating cloud provider env failed!"
    exit 1
  fi
  echo "Sleeping for $try seconds"
  sleep ${try}s
done

set -e
cd -

# Create kubeadm cluster and install machine-controller onto it
echo "Creating kubeadm cluster and install machine-controller onto it."
export E2E_SSH_PUBKEY="$(cat ~/.ssh/id_rsa.pub)"
./test/tools/integration/provision_master.sh

# Run e2e test
echo "Running e2e test."
export KUBECONFIG=$GOPATH/src/github.com/kubermatic/machine-controller/.kubeconfig
EXTRA_ARGS=""
if [[ $# -gt 0 ]]; then
  EXTRA_ARGS="-run $1"
fi
go test -race -tags=e2e -parallel 240 -v -timeout 30m  ./test/e2e/... -identifier=$BUILD_ID $EXTRA_ARGS
