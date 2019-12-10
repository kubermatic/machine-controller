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

cd $(dirname $0)

export ADDR=$(terraform output -json|jq '.ip.value' -r)


ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }

for try in {1..100}; do
  if ssh_exec "systemctl stop apt-daily apt-daily-upgrade && systemctl mask apt-daily apt-daily-upgrade && exit"; then break; fi;
  sleep 1;
done

if [[ "${1:-deploy_machine_controller}"  != "do-not-deploy-machine-controller" ]]; then
rsync -avR  -e "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no" \
    ../../.././{Makefile,examples/machine-controller.yaml,machine-controller,machine-controller-userdata-centos,machine-controller-userdata-coreos,machine-controller-userdata-ubuntu,Dockerfile,webhook} \
    root@$ADDR:/root/
fi

for try in {1..20}; do
if cat <<EOEXEC |ssh_exec
set -ex

echo "$E2E_SSH_PUBKEY" >> .ssh/authorized_keys


# Hetzner's Ubuntu Bionic comes with swap pre-configured, so we force it off.
systemctl mask swap.target
swapoff -a

if ! which make; then
  apt update
  apt install make
fi
if ! which docker; then
  apt update
  apt install -y docker.io
  systemctl enable docker.service
  systemctl start docker
  systemctl status docker
fi
if ! which kubelet; then
  apt-get update && apt-get install -y apt-transport-https
  curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
  cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
  deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF
  apt-get update
  apt-get install -y kubelet=1.17.0-00 kubeadm=1.17.0-00 kubectl=1.17.0-00
  kubeadm init --kubernetes-version=v1.17.0 --apiserver-advertise-address=$ADDR --pod-network-cidr=10.244.0.0/16 --service-cidr=172.16.0.0/12
fi
if ! ls \$HOME/.kube/config; then
  mkdir -p \$HOME/.kube
  cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config
  kubectl taint nodes --all node-role.kubernetes.io/master-
fi
if ! ls kube-flannel.yml; then
  curl -LO https://raw.githubusercontent.com/coreos/flannel/960b3243b9a7faccdfe7b3c09097105e68030ea7/Documentation/kube-flannel.yml
  kubectl apply -f kube-flannel.yml
fi

if ! grep -q kubectl /root/.bashrc; then
  echo 'function cn { kubectl config set-context \$(kubectl config current-context) --namespace=\$1; }' >> /root/.bashrc
  echo 'source <(kubectl completion bash)' >> /root/.bashrc
  echo 'alias k=kubectl' >> /root/.bashrc
  echo 'source <(k completion bash | sed s/kubectl/k/)' >> /root/.bashrc
  function cn { kubectl config set-context \$(kubectl config current-context) --namespace=\$1; }
  cn kube-system
fi

if [[ "${1:-deploy_machine_controller}"  == "do-not-deploy-machine-controller" ]]; then
  exit 0
fi
if ! ls machine-controller-deployed; then
  docker build -t kubermatic/machine-controller:latest .
  # The 10 minute window given by default for the node to appear is too short
  # when we upgrade the instance during the upgrade test
  if [[ ${JOB_NAME:-} = "pull-machine-controller-e2e-ubuntu-upgrade" ]]; then
    sed -i '/.*join-cluster-timeout=.*/d' examples/machine-controller.yaml
  fi
  sed -i -e 's/-worker-count=5/-worker-count=50/g' examples/machine-controller.yaml
  make deploy
  touch machine-controller-deployed
fi

for try in {1..10}; do
  if kubectl get pods -n kube-system|egrep '^machine-controller'|grep -v webhook|grep Running; then
    echo "Success!"
    exit 0
  fi
  sleep 10s
done

echo "Error: machine-controller didn't come up within 100 seconds!"
echo "Logs:"
kubectl logs -n kube-system \$(kubectl get pods -n kube-system|egrep '^machine-controller'|awk '{ print \$1}')
exit 1
EOEXEC
then break; fi
  sleep ${try}s
done

for try in {1..20}; do
scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
  root@$ADDR:/root/.kube/config \
  $(go env GOPATH)/src/github.com/kubermatic/machine-controller/.kubeconfig
  if [[ $? == 0 ]]; then break; fi
  sleep ${try}s
done
