CIRCLE_BUILD_NUM ?= local

USER ?= circleCI

export PATH := $(shell pwd):$(PATH)

ifeq ($(MAKECMDGOALS),apply)
	EXTRA_ARG = -auto-approve
else ifeq ($(MAKECMDGOALS),destroy)
	EXTRA_ARG = -force
endif

terraform:
	@if ! which terraform; then \
		curl https://releases.hashicorp.com/terraform/0.11.7/terraform_0.11.7_linux_amd64.zip > /tmp/terraform.zip && \
		unzip -n /tmp/terraform.zip terraform; \
	fi

ssh-key:
	if ! ls ~/.ssh/id_rsa.pub; then \
		mkdir -p ~/.ssh && \
		echo $(E2E_SSH_PUBKEY) > ~/.ssh/id_rsa.pub; \
	fi

.terraform: terraform
	terraform init >/dev/null 2>&1

.PHONY: plan apply destroy
plan apply destroy: .terraform ssh-key
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
