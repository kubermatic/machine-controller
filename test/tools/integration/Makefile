CIRCLE_BUILD_NUM ?= local

USER ?= circleCI

export PATH := $(shell pwd):$(PATH)

ifeq ($(MAKECMDGOALS),apply)
	EXTRA_ARG = -auto-approve
else ifeq ($(MAKECMDGOALS),destroy)
	EXTRA_ARG = -force
endif

.plugins/terraform-provider-hcloud:
	mkdir -p $(dir $@)
	curl -L https://github.com/hetznercloud/terraform-provider-hcloud/releases/download/v1.1.0/terraform-provider-hcloud_v1.1.0_linux_amd64.zip > /tmp/provider-hcloud.zip
	unzip -n /tmp/provider-hcloud.zip terraform-provider-hcloud
	mv terraform-provider-hcloud $@

terraform: .plugins/terraform-provider-hcloud
	@if ! which terraform; then \
		curl https://releases.hashicorp.com/terraform/0.11.3/terraform_0.11.3_linux_amd64.zip > /tmp/terraform.zip && \
		unzip -n /tmp/terraform.zip terraform; \
	fi

.terraform: terraform
	terraform init -plugin-dir .plugins >/dev/null 2>&1

.PHONY: plan apply destroy
plan apply destroy: .terraform
	@terraform $@\
		$(EXTRA_ARG) \
		-var hcloud_token=$(HZ_E2E_TOKEN) \
		-var hcloud_sshkey_name=$(USER)-$(CIRCLE_BUILD_NUM) \
		-var hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		-var hcloud_test_server_name="machine-controller-test-$(CIRCLE_BUILD_NUM)"

provision: apply
	cd ../verify/ && make verify
	cd ../../../ && make machine-controller
	./provision_master.sh
	./create-and-destroy-machine.sh
