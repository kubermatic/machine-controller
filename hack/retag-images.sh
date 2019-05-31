#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage:"
    echo "./retag-images.sh registry kubernetes-version"
    echo "Example:"
    echo "./retag-images.sh 192.168.1.1:5000 v1.14.1"
    exit 1
fi

REGISTRY=${1}
KUBERNETES_VERSION=${2}

docker pull "k8s.gcr.io/pause:3.1"
docker tag "k8s.gcr.io/pause:3.1" "${REGISTRY}/machine-controller/pause:3.1"
docker push "${REGISTRY}/machine-controller/pause:3.1"

docker pull "k8s.gcr.io/hyperkube-amd64:${KUBERNETES_VERSION}"
docker tag "k8s.gcr.io/hyperkube-amd64:${KUBERNETES_VERSION}" "${REGISTRY}/machine-controller/hyperkube-amd64:${KUBERNETES_VERSION}"
docker push "${REGISTRY}/machine-controller/hyperkube-amd64:${KUBERNETES_VERSION}"
