terraform-image:
	docker build -t terraform-hetzner .

plan:
	@docker run --rm \
		--workdir=/opt/hetzner \
		-v $(shell pwd):/opt/hetzner:rw \
		-e TF_VAR_hcloud_token=$(shell vault read --field=token work/hetzner_cloud) \
		-e TF_VAR_hcloud_sshkey_name=$(shell echo $$USER) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		terraform-hetzner \
		/bin/bash -c "if ! ls .terraform; then terraform init; fi; terraform plan"

apply:
	@docker run --rm \
		--workdir=/opt/hetzner \
		-v $(shell pwd):/opt/hetzner:rw \
		-e TF_VAR_hcloud_token=$(shell vault read --field=token work/hetzner_cloud) \
		-e TF_VAR_hcloud_sshkey_name=$(shell echo $$USER) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		terraform-hetzner \
		/bin/bash -c "terraform apply -auto-approve"

destroy:
	@docker run --rm \
		--workdir=/opt/hetzner \
		-v $(shell pwd):/opt/hetzner:rw \
		-e TF_VAR_hcloud_token=$(shell vault read --field=token work/hetzner_cloud) \
		-e TF_VAR_hcloud_sshkey_name=$(shell echo $$USER) \
		-e TF_VAR_hcloud_sshkey_content="$(shell cat ~/.ssh/id_rsa.pub)" \
		terraform-hetzner \
		/bin/bash -c "terraform destroy -force"
