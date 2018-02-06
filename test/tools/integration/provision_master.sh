#!/usr/bin/env bash

set -euo pipefail

export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')


ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }

until ssh_exec exit; do sleep 1; done

cat <<EOEXEC |ssh_exec
set -ex

if ! which docker; then
  apt update
  apt install -y docker.io
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
  apt-get install -y kubelet kubeadm kubectl
  kubeadm init --apiserver-advertise-address=$ADDR --pod-network-cidr=10.244.0.0/16
fi
if ! ls \$HOME/.kube/config; then
  mkdir -p \$HOME/.kube
  cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config
  kubectl taint nodes --all node-role.kubernetes.io/master-
fi
if ! ls kube-flannel.yml; then
  curl -LO https://raw.githubusercontent.com/coreos/flannel/v0.9.1/Documentation/kube-flannel.yml
  kubectl apply -f kube-flannel.yml
fi
EOEXEC
