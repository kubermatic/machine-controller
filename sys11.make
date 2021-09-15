REGISTRY_NAMESPACE=syseleven
TAG=$(if $(GITHUB_REF),$(GITHUB_REF),latest)

.PHONY: test
test:
	$(MAKE) test-unit-docker

.PHONY: ci-tag-and-push-image
ci-tag-and-push-image:
	echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin
	$(MAKE) REGISTRY_NAMESPACE=$(REGISTRY_NAMESPACE) IMAGE_TAG=$(TAG) docker-image-publish
