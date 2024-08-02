OUTPUT_FILE = driver.yaml
REPO_NAME = mc-ccm-aws
CHART_VERSION = 0.0.8

.PHONY: default
default: setup-helm build clean-helm

.PHONY: build
build:
	helm --namespace kube-system template aws-cloud-controller-manager $(REPO_NAME)/aws-cloud-controller-manager \
	  --version $(CHART_VERSION) \
	  --skip-tests \
	  > $(OUTPUT_FILE)

.PHONY: setup-helm
setup-helm:
	helm repo add --force-update $(REPO_NAME) https://kubernetes.github.io/cloud-provider-aws
	helm repo update $(REPO_NAME)

.PHONY: clean-helm
clean-helm:
	helm repo remove $(REPO_NAME)
