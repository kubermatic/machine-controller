USE_VOLUME_CONTAINER ?= false
CIRCLE_BUILD_NUM ?= local

USER ?= circleCI

# Required because circlecCI can not use
# Docker volumes, so we have to use a volume
# container instead there
ifeq ($(USE_VOLUME_CONTAINER),true)
	VOL_ARG = --volumes-from vol-container
else
	VOL_ARG = -v $(shell pwd):/opt/hetzner:rw
endif

terraform-image:
	docker build -t terraform-hetzner .

plan:
	@docker run --rm \
		--workdir=/opt/hetzner \
		$(VOL_ARG) \
		-e TF_VAR_hcloud_token=$(HZ_TOKEN) \
		-e TF_VAR_hcloud_sshkey_name=$(USER)-$(CIRCLE_BUILD_NUM) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		-e TF_VAR_hcloud_test_server_name="machine-controller-test-$(CIRCLE_BUILD_NUM)" \
		terraform-hetzner \
		/bin/bash -c "if ! ls .terraform; then terraform init; fi; terraform plan"

apply:
	@docker run --rm \
		--workdir=/opt/hetzner \
		$(VOL_ARG) \
		-e TF_VAR_hcloud_token=$(HZ_TOKEN) \
		-e TF_VAR_hcloud_sshkey_name=$(USER)-$(CIRCLE_BUILD_NUM) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		-e TF_VAR_hcloud_test_server_name="machine-controller-test-$(CIRCLE_BUILD_NUM)" \
		terraform-hetzner \
		/bin/bash -c "if ! ls .terraform; then terraform init; fi; terraform apply -auto-approve"

provision: apply
	cd ../verify/ && make verify
	cd ../../../ && make machine-controller
	./provision_master.sh
	./create-and-destroy-machine.sh

destroy:
	@docker run --rm \
		--workdir=/opt/hetzner \
		$(VOL_ARG) \
		-e TF_VAR_hcloud_token=$(HZ_TOKEN) \
		-e TF_VAR_hcloud_sshkey_name=$(USER)-$(CIRCLE_BUILD_NUM) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		-e TF_VAR_hcloud_test_server_name="machine-controller-test-$(CIRCLE_BUILD_NUM)" \
		terraform-hetzner \
		/bin/bash -c "/usr/local/bin/terraform destroy -force"
