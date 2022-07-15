# Copyright 2021 The Kubernetes Authors.
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

# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

.DEFAULT_GOAL:=help
SHELL:=/usr/bin/env bash

COLOR:=\\033[36m
NOCOLOR:=\\033[0m

# Set version variables for LDFLAGS
GIT_VERSION ?= $(shell git describe --tags --always --dirty)
GIT_HASH ?= $(shell git rev-parse HEAD)
DATE_FMT = +%Y-%m-%dT%H:%M:%SZ
SOURCE_DATE_EPOCH ?= $(shell git log -1 --pretty=%ct)
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif
GIT_TREESTATE = "clean"
DIFF = $(shell git diff --quiet >/dev/null 2>&1; if [ $$? -eq 1 ]; then echo "1"; fi)
ifeq ($(DIFF), 1)
    GIT_TREESTATE = "dirty"
endif

LDFLAGS=-buildid= -X sigs.k8s.io/release-utils/version.gitVersion=$(GIT_VERSION) \
        -X sigs.k8s.io/release-utils/version.gitCommit=$(GIT_HASH) \
        -X sigs.k8s.io/release-utils/version.gitTreeState=$(GIT_TREESTATE) \
        -X sigs.k8s.io/release-utils/version.buildDate=$(BUILD_DATE)

KO_DOCKER_REPO ?= ghcr.io/kubernetes-sigs

build: ## Build zeitgeist
	go build -trimpath -ldflags "$(LDFLAGS)"

ko-local: ## Build zeitgeist image locally (does not push it)
	LDFLAGS="$(LDFLAGS)" \
	ko build --local --tags $(GIT_VERSION),latest --base-import-paths --platform=all .

.PHONY: snapshot
snapshot: ## Build zeitgeist binaries with goreleaser in snapshot mode
	LDFLAGS="$(LDFLAGS)" GIT_HASH=$(GIT_HASH) GIT_VERSION=$(GIT_VERSION) \
	goreleaser release --rm-dist --snapshot --skip-sign --skip-publish

lint:
	test -z $(shell go fmt .) || (echo "Linting failed !" && exit 8)
	go vet ./...
	go get -u golang.org/x/lint/golint
	golint ./...

test: ## Runs unit testing
	go test ./... -covermode=count -coverprofile=coverage.out

test-results: test
	go tool cover -html=coverage.out

generate: ## Generate go code for the fake clients
	go generate ./...

verify: verify-boilerplate verify-golangci-lint verify-go-mod  ## Runs verification scripts to ensure correct execution

verify-boilerplate: ## Runs the file header check
	./hack/verify-boilerplate.sh

verify-go-mod: ## Runs the go module linter
	./hack/verify-go-mod.sh

verify-golangci-lint: ## Runs all golang linters
	./hack/verify-golangci-lint.sh

## Release

.PHONY: goreleaser
goreleaser: ## Build zeitgeist binaries with goreleaser
	LDFLAGS="$(LDFLAGS)" GIT_HASH=$(GIT_HASH) GIT_VERSION=$(GIT_VERSION) \
	goreleaser release --rm-dist

.PHONY: ko-release
ko-release: ## Build zeitgeist image
	LDFLAGS="$(LDFLAGS)" GIT_HASH=$(GIT_HASH) GIT_VERSION=$(GIT_VERSION) \
	ko build --base-import-paths \
	--platform=all --tags $(GIT_VERSION),$(GIT_HASH),latest --image-refs imagerefs .

imagerefs := $(shell cat imagerefs testimagerefs)
sign-refs := $(foreach ref,$(imagerefs),$(ref))
.PHONY: sign-images
sign-images:
	cosign sign -a GIT_TAG=$(GIT_VERSION) -a GIT_HASH=$(GIT_HASH) $(sign-refs)

##@ Helpers

.PHONY: help

help:  ## Display this help
	@awk \
		-v "col=${COLOR}" -v "nocol=${NOCOLOR}" \
		' \
			BEGIN { \
				FS = ":.*##" ; \
				printf "\nUsage:\n  make %s<target>%s\n", col, nocol \
			} \
			/^[a-zA-Z_-]+:.*?##/ { \
				printf "  %s%-15s%s %s\n", col, $$1, nocol, $$2 \
			} \
			/^##@/ { \
				printf "\n%s%s%s\n", col, substr($$0, 5), nocol \
			} \
		' $(MAKEFILE_LIST)
