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

SHELL := /bin/bash

BUILD_ID ?= $(USER)-local

USER ?= prow

export PATH := $(shell pwd):$(PATH)

ifeq ($(MAKECMDGOALS),apply)
	EXTRA_ARG = -auto-approve
endif

.PHONY: terraform
terraform:
	@if ! which terraform; then \
		curl https://releases.hashicorp.com/terraform/1.1.3/terraform_1.1.3_linux_amd64.zip \
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
	make -C ../../../ clean
	make -C ../../../ all GOOS=linux
	./provision_master.sh
