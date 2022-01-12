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

K8S_VERSION=1.23.0
echo "$LC_E2E_SSH_PUBKEY" >> .ssh/authorized_keys

# Hetzner's Ubuntu Bionic comes with swap pre-configured, so we force it off.
systemctl mask swap.target
swapoff -a

if ! which buildah; then
  sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_20.04/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list"
  wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/xUbuntu_20.04/Release.key -O Release.key
  apt-key add - < Release.key
  apt-get update
  apt-get -y install buildah
fi
if ! which make; then
  apt update
  apt install make
fi
if ! which containerd; then
  apt update
  apt-get install -y apt-transport-https ca-certificates curl software-properties-common lsb-release
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
  add-apt-repository "deb https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

  cat <<EOF | tee /etc/crictl.yaml
  runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

  mkdir -p /etc/systemd/system/containerd.service.d
  cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
  [Service]
  Restart=always
  EnvironmentFile=-/etc/environment
EOF

  DEBIAN_FRONTEND=noninteractive apt-get install -y  containerd.io=1.4*
  apt-mark hold containerd.io

  mkdir -p /etc/containerd/ && touch /etc/containerd/config.toml
  cat <<EOF | tee /etc/containerd/config.toml
  version = 2

  [metrics]
  address = "127.0.0.1:1338"

  [plugins]
  [plugins."io.containerd.grpc.v1.cri"]
  [plugins."io.containerd.grpc.v1.cri".containerd]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
  [plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
  endpoint = ["https://registry-1.docker.io"]
EOF

  systemctl daemon-reload
  systemctl enable --now containerd
  systemctl restart containerd.service

  cat <<EOF | sudo tee /etc/modules-load.d/containerd.conf
  overlay
  br_netfilter
EOF
  modprobe overlay
  modprobe br_netfilter

  cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf
  net.bridge.bridge-nf-call-iptables  = 1
  net.ipv4.ip_forward                 = 1
  net.bridge.bridge-nf-call-ip6tables = 1
EOF

  sysctl --system
fi
if ! which kubelet; then
  apt-get update && apt-get install -y apt-transport-https
  curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
  cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
  deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF
  apt-get update
  apt-get install -y \
      kubelet=${K8S_VERSION}-00 \
      kubeadm=${K8S_VERSION}-00 \
      kubectl=${K8S_VERSION}-00
  kubeadm init --kubernetes-version=${K8S_VERSION} \
    --apiserver-advertise-address=${LC_ADDR} --pod-network-cidr=10.244.0.0/16 --service-cidr=172.16.0.0/12
fi
if ! ls $HOME/.kube/config; then
  mkdir -p $HOME/.kube
  cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  kubectl taint nodes --all node-role.kubernetes.io/master-
fi
if ! ls kube-flannel.yml; then
  kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/v0.15.0/Documentation/kube-flannel.yml
fi

if ! grep -q kubectl /root/.bashrc; then
  cat << 'EOF' >> /root/.bashrc
function cn { kubectl config set-context $(kubectl config current-context) --namespace=$1; }
source <(kubectl completion bash)
alias k=kubectl
source <(k completion bash | sed s/kubectl/k/)
EOF
  function cn { kubectl config set-context $(kubectl config current-context) --namespace=$1; }
  cn kube-system
fi

if [[ "${LC_DEPLOY_MACHINE:-}"  == "do-not-deploy-machine-controller" ]]; then
  exit 0
fi
if ! ls machine-controller-deployed; then
  buildah build-using-dockerfile --format docker --file Dockerfile --tag kubermatic/machine-controller:latest
  mkdir "images"
  buildah push localhost/kubermatic/machine-controller oci-archive:./images/machine-controller.tar:localhost/kubermatic/machine-controller:latest
  ctr --debug --namespace=k8s.io images import --all-platforms --no-unpack images/machine-controller.tar
  sed -i "s_- image: kubermatic/machine-controller:latest_- image: localhost/kubermatic/machine-controller:latest_g" examples/machine-controller.yaml
  # The 10 minute window given by default for the node to appear is too short
  # when we upgrade the instance during the upgrade test
  if [[ ${LC_JOB_NAME:-} = "pull-machine-controller-e2e-ubuntu-upgrade" ]]; then
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
kubectl logs -n kube-system $(kubectl get pods -n kube-system|egrep '^machine-controller'|awk '{ print $1}')
exit 1