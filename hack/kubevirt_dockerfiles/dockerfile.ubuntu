FROM kubevirt/registry-disk-v1alpha:v0.10.0

RUN curl -L -o /disk/bionic.img https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img && \
  qemu-img resize /disk/bionic.img +10g
