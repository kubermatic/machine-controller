#!/bin/bash

if [ ! -x "$(which wget)" ]; then
  echo "wget not found"
  exit 1
fi

if [ ! -x "$(which bash)" ]; then
  echo "bash not found"
  exit 1
fi

if [ ! -x "$(which 7z)" ]; then
  echo "7z not found"
  exit 1
fi

if [ ! -x "$(which packer)" ]; then
  echo "packer not found"
  exit 1
fi

if [ ! -x "$(which packer-provisioner-windows-update)" ]; then
  echo "packer-provisioner-windows-update not found."
  echo "Download from: https://github.com/rgl/packer-provisioner-windows-update/releases"
  exit 1
fi

if [ ! -x "$(which envsubst)" ]; then
  echo "envsubst not found"
  exit 1
fi

if [ ! -x "$(which qemu-img)" ]; then
  echo "qemu-img not found"
  exit 1
fi
