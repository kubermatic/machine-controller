#!/usr/bin/env bash

set -e

export BUILD_ID="${BUILD_ID:-$CIRCLE_BUILD_NUM}"

# Install dependencies
echo "Installing dependencies."
apt update && apt install -y jq rsync unzip &&
curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.10.0/bin/linux/amd64/kubectl &&
chmod +x kubectl &&
mv kubectl /usr/local/bin

# Generate ssh keypair
echo "Generating ssh keypairs."
ssh-keygen -f $HOME/.ssh/id_rsa -P ''

# Create environment at cloud provider
echo "Creating environment at cloud provider."
make -C test/tools/integration apply

# Create kubeadm cluster and install machine-controller onto it
echo "Creating kubeadm cluster and install machine-controller onto it."
./test/tools/integration/provision_master.sh

# Run e2e test
echo "Running e2e test."
export KUBECONFIG=$GOPATH/src/github.com/kubermatic/machine-controller/.kubeconfig &&
go test -race -tags=e2e -parallel 240 -v -timeout 30m  ./test/e2e/... -identifier=$BUILD_ID

function cleanup {
    set +e

    # Clean up machines
    echo "Cleaning up machines."
    ./test/tools/integration/cleanup_machines.sh

    # Clean up master
    echo "Clean up master."
    make -C test/tools/integration destroy
}
trap cleanup EXIT
