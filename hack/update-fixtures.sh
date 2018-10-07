#!/usr/bin/env bash

go test ./... -update || go test ./...


if [[ $? -eq 0 ]]; then echo "Successfully updated fixtures"; else "Failed to update fixtures"; fi
