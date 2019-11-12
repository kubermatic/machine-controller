REGISTRY_NAMESPACE=syseleven
TAG=$(if $(TRAVIS_TAG),$(TRAVIS_TAG),latest)

compile:
	$(MAKE) machine-controller-docker webhook-docker

ci-tag-and-push-image:
	echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin
	$(MAKE) REGISTRY_NAMESPACE=$(REGISTRY_NAMESPACE) IMAGE_TAG=$(TAG) docker-image-nodep
