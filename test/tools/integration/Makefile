SHELL := /bin/bash

BUILD_ID ?= $(USER)-local

USER ?= prow

export PATH := $(shell pwd):$(PATH)

ifeq ($(MAKECMDGOALS),apply)
	EXTRA_ARG = -auto-approve
else ifeq ($(MAKECMDGOALS),destroy)
	EXTRA_ARG = -force
endif

.PHONY: terraform
terraform:
	@if ! which terraform; then \
		curl https://releases.hashicorp.com/terraform/0.11.11/terraform_0.11.11_linux_amd64.zip \
			--retry 5 \
			-o	/tmp/terraform.zip && \
		unzip -n /tmp/terraform.zip terraform && \
		mv terraform /usr/local/bin; \
	fi

.terraform: terraform
	@if ! ls .terraform &>/dev/null; then \
		terraform init &>/dev/null; \
	fi

.PHONY: plan apply destroy
plan apply destroy: .terraform
	@terraform $@\
		$(EXTRA_ARG) \
		-var hcloud_token=$(HZ_E2E_TOKEN) \
		-var hcloud_sshkey_name=$(USER)-$(BUILD_ID) \
		-var hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		-var hcloud_test_server_name="machine-controller-test-$(BUILD_ID)"

provision: apply
	cd ../verify/ && make verify
	cd ../../../ && make machine-controller
	./provision_master.sh
	./create-and-destroy-machine.sh
