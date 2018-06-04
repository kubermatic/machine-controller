#!/usr/bin/env bash

set -euo pipefail
set -x

cd $(dirname $0)

export ADDR=$(cat terraform.tfstate |jq -r '.modules[0].resources["hcloud_server.machine-controller-test"].primary.attributes.ipv4_address')


ssh_exec() { ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@$ADDR $@; }

for try in {1..100}; do
  if ssh_exec "systemctl stop apt-daily apt-daily-upgrade && systemctl mask apt-daily apt-daily-upgrade && exit"; then break; fi;
  sleep 1;
done


rsync -av  -e "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no" \
    ../../../{examples/machine-controller.yaml,machine-controller,Dockerfile} \
    root@$ADDR:/root/

cat <<EOEXEC |ssh_exec
set -ex

if ! grep -q kubectl /root/.bashrc; then
  echo 'function cn { kubectl config set-context \$(kubectl config current-context) --namespace=\$1; }' >> /root/.bashrc
  echo 'source <(kubectl completion bash)' >> /root/.bashrc
fi

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
  curl -LO https://raw.githubusercontent.com/coreos/flannel/v0.10.0/Documentation/kube-flannel.yml
  kubectl apply -f kube-flannel.yml
fi
if ! ls machine-controller-deployed; then
  docker build -t kubermatic/machine-controller:latest .
  kubectl apply -f machine-controller.yaml
  touch machine-controller-deployed
fi

for try in {1..10}; do
  if kubectl get pods -n kube-system|egrep '^machine-controller'|grep Running; then
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

scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
  root@$ADDR:/root/.kube/config \
  $(go env GOPATH)/src/github.com/kubermatic/machine-controller/.kubeconfig
