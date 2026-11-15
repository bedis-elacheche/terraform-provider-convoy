default: fmt lint build test

TF ?= $(shell command -v terraform)

build:
	go build -v ./...

install: build
	go install -v ./...

fmt:
	gofmt -s -w -e .
	terraform fmt -recursive examples

lint:
	go vet ./...

# Unit tests plus end-to-end Terraform tests against the in-memory fake Convoy
# API (internal/convoytest). Needs a terraform binary (>= 1.14 for actions).
test:
	TF_ACC_TERRAFORM_PATH=$(TF) CHECKPOINT_DISABLE=1 go test -count=1 -timeout=10m ./...

# Acceptance tests against a real Convoy instance:
#   CONVOY_API_KEY=... CONVOY_PROJECT_ID=... [CONVOY_BASE_URL=...] make testacc
testacc:
	TF_ACC=1 TF_ACC_TERRAFORM_PATH=$(TF) go test -count=1 -timeout=30m -run TestAccRealConvoy ./internal/provider/

docs:
	go generate ./...

.PHONY: default build install fmt lint test testacc docs
