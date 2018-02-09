# vim: tw=180
FROM debian:stretch

RUN \
  apt update && \
  apt install -y unzip curl && \
  cd /tmp && \
  curl -LO https://releases.hashicorp.com/terraform/0.11.3/terraform_0.11.3_linux_amd64.zip && \
  curl -LO https://github.com/hetznercloud/terraform-provider-hcloud/releases/download/v1.0.0/terraform-provider-hcloud_v1.0.0_linux_amd64.zip && \
  unzip terraform_0.11.3_linux_amd64.zip && \
  unzip terraform-provider-hcloud_v1.0.0_linux_amd64.zip && \
  mkdir -p ~/.terraform.d/plugins/ && \
  mv terraform-provider-hcloud ~/.terraform.d/plugins/ && \
  mv terraform /usr/local/bin/
