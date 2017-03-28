.PHONY: all binary build build-binary build-gccgo bundles cross default docs gccgo test test-integration-cli test-unit validate help win tgz

# set the graph driver as the current graphdriver if not set
DRIVER := $(if $(STORAGE_DRIVER),$(STORAGE_DRIVER),$(if $(DOCKER_GRAPHDRIVER),DOCKER_GRAPHDRIVER),$(shell docker info 2>&1 | grep "Storage Driver" | sed 's/.*: //'))

GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
EPOCH_TEST_COMMIT := 0418ebf59f9e1f564831c0ba9378b7f8e40a1c73
SYSTEM_GOPATH := ${GOPATH}

RUNINVM := vagrant/runinvm.sh

default all: build ## validate all checks, build linux binaries, run all tests\ncross build non-linux binaries and generate archives\nusing VMs
	$(RUNINVM) hack/make.sh

build build-binary: bundles ## build using go on the host
	hack/make.sh binary

build-gccgo: bundles ## build using gccgo on the host
	hack/make.sh gccgo

binary: bundles
	$(RUNINVM) hack/make.sh binary

bundles:
	mkdir -p bundles

cross: build ## cross build the binaries for darwin, freebsd and windows\nusing VMs
	$(RUNINVM) hack/make.sh binary cross

win: build ## cross build the binary for windows using VMs
	$(RUNINVM) hack/make.sh win

tgz: build ## build the archives (.zip on windows and .tgz otherwise)\ncontaining the binaries on the host
	hack/make.sh binary cross tgz

docs: ## build the docs on the host
	$(MAKE) -C docs docs

gccgo: build-gccgo ## build the gcc-go linux binaries using VMs
	$(RUNINVM) hack/make.sh gccgo

test: build ## run the unit and integration tests using VMs
	$(RUNINVM) hack/make.sh binary cross test-unit

test-unit: build ## run the unit tests using VMs
	$(RUNINVM) hack/make.sh test-unit

validate: build ## validate DCO, Seccomp profile generation, gofmt,\n./pkg/ isolation, golint, tests, tomls, go vet and vendor\nusing VMs
	$(RUNINVM) hack/make.sh validate-dco validate-gofmt validate-pkg validate-lint validate-test validate-toml validate-vet

lint:
	@which gometalinter > /dev/null 2>/dev/null || (echo "ERROR: gometalinter not found. Consider 'make install.tools' target" && false)
	@echo "checking lint"
	@./.tool/lint

.PHONY: .gitvalidation
# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make install.tools' target" && false)
ifeq ($(TRAVIS_EVENT_TYPE),pull_request)
	git-validation -q -run DCO,short-subject
else ifeq ($(TRAVIS_EVENT_TYPE),push)
	git-validation -q -run DCO,short-subject -no-travis -range $(EPOCH_TEST_COMMIT)..$(TRAVIS_BRANCH)
else
	git-validation -q -run DCO,short-subject -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: install.tools

install.tools: .install.gitvalidation .install.gometalinter .install.md2man

.install.gitvalidation:
	GOPATH=${SYSTEM_GOPATH} go get github.com/vbatts/git-validation

.install.gometalinter:
	GOPATH=${SYSTEM_GOPATH} go get github.com/alecthomas/gometalinter
	GOPATH=${SYSTEM_GOPATH} gometalinter --install

.install.md2man:
	GOPATH=${SYSTEM_GOPATH} go get github.com/cpuguy83/go-md2man

help: ## this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-z A-Z_-]+:.*?## / {gsub(" ",",",$$1);gsub("\\\\n",sprintf("\n%22c"," "), $$2);printf "\033[36m%-21s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

