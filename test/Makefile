tools/verify/verify:
	cd tools/verify && make verify

hetzner-e2e: tools/verify/verify
	cd tools/integration && \
		make provision && \
		make destroy
