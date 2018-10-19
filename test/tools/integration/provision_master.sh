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

rsync -avR  -e "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no" \
    ../../.././{Makefile,examples/machine-controller.yaml,machine-controller,Dockerfile,webhook} \
    root@$ADDR:/root/

cat <<EOEXEC |ssh_exec
set -ex

echo "$E2E_SSH_PUBKEY" >> .ssh/authorized_keys

if ! grep -q kubectl /root/.bashrc; then
  echo 'function cn { kubectl config set-context \$(kubectl config current-context) --namespace=\$1; }' >> /root/.bashrc
  echo 'source <(kubectl completion bash)' >> /root/.bashrc
fi

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
  apt-get install -y kubelet=1.12.0-00 kubeadm=1.12.0-00 kubectl=1.12.0-00
  kubeadm init --kubernetes-version=v1.12.0 --apiserver-advertise-address=$ADDR --pod-network-cidr=10.244.0.0/16
  sed -i 's/\(.*leader-elect=true\)/\1\n    - --feature-gates=ScheduleDaemonSetPods=false/g' /etc/kubernetes/manifests/kube-scheduler.yaml
  sed -i 's/\(.*leader-elect=true\)/\1\n    - --feature-gates=ScheduleDaemonSetPods=false/g' /etc/kubernetes/manifests/kube-controller-manager.yaml
fi
if ! ls \$HOME/.kube/config; then
  mkdir -p \$HOME/.kube
  cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config
  kubectl taint nodes --all node-role.kubernetes.io/master-
  kubectl get configmap -n kube-system kubelet-config-1.12 -o yaml \
   |sed '/creationTimestamp/d;/resourceVersion/d;/selfLink/d;/uid/d;s/kubelet-config-1.12/kubelet-config-1.11/g' \
   |kubectl apply -f -
fi
if ! ls kube-flannel.yml; then
  # Open upstream PR for the fix that is needed for kube 1.12: https://github.com/coreos/flannel/pull/1045
  curl -LO https://raw.githubusercontent.com/alvaroaleman/flannel/kube-1.12-support/Documentation/kube-flannel.yml
  kubectl apply -f kube-flannel.yml
fi

if [[ "${1:-deploy_machine_controller}"  == "do-not-deploy-machine-controller" ]]; then
  exit 0
fi
if ! ls machine-controller-deployed; then
  docker build -t kubermatic/machine-controller:latest .
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

scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
  root@$ADDR:/root/.kube/config \
  $(go env GOPATH)/src/github.com/kubermatic/machine-controller/.kubeconfig
