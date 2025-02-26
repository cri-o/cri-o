GO ?= go

TRIMPATH ?= -trimpath
GO_ARCH=$(shell $(GO) env GOARCH)
GO_BUILD ?= $(GO) build $(TRIMPATH)
GO_TEST ?= $(GO) test $(TRIMPATH)
GO_RUN ?= $(GO) run
NIX_IMAGE ?= nixos/nix:2.25.4

PROJECT := github.com/cri-o/cri-o
CRIO_INSTANCE := crio_dev
PREFIX ?= ${DESTDIR}/usr/local
BINDIR ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR ?= ${PREFIX}/share/man
ETCDIR ?= ${DESTDIR}/etc
ETCDIR_CRIO ?= ${ETCDIR}/crio
DATAROOTDIR ?= ${PREFIX}/share/containers
BUILDTAGS ?= containers_image_ostree_stub \
			 $(shell hack/apparmor_tag.sh) \
			 $(shell hack/btrfs_installed_tag.sh) \
			 $(shell hack/btrfs_tag.sh) \
			 $(shell hack/openpgp_tag.sh) \
			 $(shell hack/seccomp_tag.sh) \
			 $(shell hack/selinux_tag.sh) \
			 $(shell hack/libsubid_tag.sh)
CRICTL_CONFIG_DIR=${DESTDIR}/etc
CONTAINER_RUNTIME ?= podman
PWD := $(shell pwd)
BUILD_PATH := ${PWD}/build
BUILD_BIN_PATH := ${BUILD_PATH}/bin
COVERAGE_PATH := ${BUILD_PATH}/coverage
TESTBIN_PATH := ${BUILD_PATH}/test
MOCK_PATH := ./test/mocks

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
FISHINSTALLDIR=${PREFIX}/share/fish/completions
ZSHINSTALLDIR=${PREFIX}/share/zsh/site-functions
OCIUMOUNTINSTALLDIR=$(PREFIX)/share/oci-umount/oci-umount.d

SELINUXOPT ?= $(shell selinuxenabled 2>/dev/null && echo -Z)

SOURCE_DATE_EPOCH ?= $(shell date +%s)

GO_MD2MAN ?= ${BUILD_BIN_PATH}/go-md2man
GINKGO := ${BUILD_BIN_PATH}/ginkgo
MOCKGEN := ${BUILD_BIN_PATH}/mockgen
GOLANGCI_LINT := ${BUILD_BIN_PATH}/golangci-lint
GOLANGCI_LINT_VERSION := v1.64.5
GO_MOD_OUTDATED := ${BUILD_BIN_PATH}/go-mod-outdated
GO_MOD_OUTDATED_VERSION := 0.9.0
GOSEC := ${BUILD_BIN_PATH}/gosec
GOSEC_VERSION := 2.21.4
MDTOC := ${BUILD_BIN_PATH}/mdtoc
MDTOC_VERSION := v1.4.0
RELEASE_NOTES := ${BUILD_BIN_PATH}/release-notes
RELEASE_NOTES_VERSION := v0.17.11
ZEITGEIST := ${BUILD_BIN_PATH}/zeitgeist
ZEITGEIST_VERSION := v0.5.4
SHFMT := ${BUILD_BIN_PATH}/shfmt
SHFMT_VERSION := v3.10.0
SHELLCHECK := ${BUILD_BIN_PATH}/shellcheck
SHELLCHECK_VERSION := v0.10.0
BATS_FILES := $(wildcard test/*.bats)

ifeq ($(shell bash -c '[[ `command -v git` && `git rev-parse --git-dir 2>/dev/null` ]] && echo true'), true)
	COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
	GIT_TREE_STATE := $(if $(shell git status --porcelain --untracked-files=no),dirty,clean)
else
	COMMIT_NO := unknown
	GIT_TREE_STATE := unknown
endif

# pass crio CLI options to generate custom configuration options at build time
CONF_OVERRIDES ?=

CROSS_BUILD_TARGETS := \
	bin/crio.cross.windows.amd64 \
	bin/crio.cross.darwin.amd64 \
	bin/crio.cross.linux.amd64

GO_FILES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

# Some of the packages use the golang testing infra in end-to-end tests.
# These can't be run as unit tests so ginkgo should skip them.
GINKGO_SKIP_PACKAGES = test/nri

# Set DEBUG=1 to enable debug symbols in binaries
DEBUG ?= 0
ifeq ($(DEBUG),0)
SHRINKFLAGS = -s -w
else
GCFLAGS = -gcflags '-N -l'
endif

DATE_FMT = +'%Y-%m-%dT%H:%M:%SZ'
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date -u "$(DATE_FMT)")
endif

BASE_LDFLAGS = ${SHRINKFLAGS} \
	-X ${PROJECT}/internal/version.buildDate=${BUILD_DATE}

GO_LDFLAGS = -ldflags '${BASE_LDFLAGS} ${EXTRA_LDFLAGS}'

define curl_to
	curl -sSfL --retry 5 --retry-delay 3 "$(1)" -o $(2)
	chmod +x $(2)
endef

all: binaries crio.conf docs

COLOR:=\\033[36m
NOCOLOR:=\\033[0m
WIDTH:=30

.PHONY: help
help:  ## Display this help.
	@awk \
		-v "col=${COLOR}" -v "nocol=${NOCOLOR}" \
		' \
			BEGIN { \
				FS = ":.*##" ; \
				printf "Usage:\n  make %s<target>%s\n", col, nocol \
			} \
			/^[./a-zA-Z_-]+:.*?##/ { \
				printf "  %s%-${WIDTH}s%s %s\n", col, $$1, nocol, $$2 \
			} \
			/^##@/ { \
				printf "\n%s\n", substr($$0, 5) \
			} \
		' $(MAKEFILE_LIST)

$(BUILD_BIN_PATH):
	mkdir -p $(BUILD_BIN_PATH)

$(GO_MD2MAN):
	hack/go-install.sh $(BUILD_BIN_PATH) go-md2man github.com/cpuguy83/go-md2man/v2@latest

$(GINKGO):
	hack/go-install.sh $(BUILD_BIN_PATH) ginkgo github.com/onsi/ginkgo/v2/ginkgo@latest

$(RELEASE_NOTES): $(BUILD_BIN_PATH)
	$(call curl_to,https://storage.googleapis.com/k8s-artifacts-sig-release/kubernetes/release/$(RELEASE_NOTES_VERSION)/release-notes-amd64-linux,$(RELEASE_NOTES))

$(SHFMT): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/mvdan/sh/releases/download/$(SHFMT_VERSION)/shfmt_$(SHFMT_VERSION)_linux_amd64,$(SHFMT))

$(ZEITGEIST): $(BUILD_BIN_PATH)
	$(call curl_to,https://storage.googleapis.com/k8s-artifacts-sig-release/kubernetes-sigs/zeitgeist/$(ZEITGEIST_VERSION)/zeitgeist-amd64-linux,$(ZEITGEIST))

$(MOCKGEN):
	hack/go-install.sh $(BUILD_BIN_PATH) mockgen go.uber.org/mock/mockgen@latest

$(GO_MOD_OUTDATED): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/psampaz/go-mod-outdated/releases/download/v$(GO_MOD_OUTDATED_VERSION)/go-mod-outdated_$(GO_MOD_OUTDATED_VERSION)_Linux_x86_64.tar.gz,$(BUILD_BIN_PATH)/gmo.tar.gz)
	tar xf $(BUILD_BIN_PATH)/gmo.tar.gz -C $(BUILD_BIN_PATH)

$(GOSEC): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/securego/gosec/releases/download/v$(GOSEC_VERSION)/gosec_$(GOSEC_VERSION)_linux_amd64.tar.gz,$(BUILD_BIN_PATH)/gosec.tar.gz)
	tar xf $(BUILD_BIN_PATH)/gosec.tar.gz -C $(BUILD_BIN_PATH)

$(MDTOC): $(BUILD_BIN_PATH)
	$(call curl_to,https://storage.googleapis.com/k8s-artifacts-sig-release/kubernetes-sigs/mdtoc/$(MDTOC_VERSION)/mdtoc-amd64-linux,$(MDTOC))

$(GOLANGCI_LINT):
	export VERSION=$(GOLANGCI_LINT_VERSION) \
		URL=https://raw.githubusercontent.com/golangci/golangci-lint \
		BINDIR=${BUILD_BIN_PATH} && \
	curl -sSfL $$URL/$$VERSION/install.sh | sh -s $$VERSION

$(SHELLCHECK): $(BUILD_BIN_PATH)
	URL=https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).linux.x86_64.tar.xz \
	SHA256SUM=f35ae15a4677945428bdfe61ccc297490d89dd1e544cc06317102637638c6deb && \
	curl -sSfL $$URL | tar xfJ - -C ${BUILD_BIN_PATH} --strip 1 shellcheck-$(SHELLCHECK_VERSION)/shellcheck && \
	sha256sum ${SHELLCHECK} | grep -q $$SHA256SUM


##@ Build targets:

.PHONY: binaries
binaries: bin/crio bin/pinns ## Build all binaries.

.PHONY: test-binaries
test-binaries: ## Build all test-binaries.
test-binaries: \
	test/copyimg/copyimg \
	test/checkseccomp/checkseccomp \
	test/checkcriu/checkcriu \
	test/nri/nri.test

bin/pinns: ## Build pinns.
	$(MAKE) -C pinns

test/copyimg/copyimg: $(GO_FILES) ## Build the compyimg test binary.
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ ./test/copyimg

test/checkseccomp/checkseccomp: $(GO_FILES) ## Build the checkseccomp test binary.
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ ./test/checkseccomp

test/checkcriu/checkcriu: $(GO_FILES) ## Build the checkcriu test binary.
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ ./test/checkcriu

test/nri/nri.test: $(wildcard test/nri/*.go) ## Build the NRI test binary.
	$(GO_TEST) $(GCFLAGS) $(GO_LDFLAGS) --tags "test $(BUILDTAGS)" -c ./test/nri -o $@

bin/crio: $(GO_FILES) ## Build the CRI-O main binary.
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ ./cmd/crio

.PHONY: build-static
build-static: ## Build the static binaries.
	$(CONTAINER_RUNTIME) run --network=host --rm --privileged -ti -v /:/mnt \
		$(NIX_IMAGE) cp -rfT /nix /mnt/nix
	$(CONTAINER_RUNTIME) run --network=host --rm --privileged -ti -v /nix:/nix -v ${PWD}:${PWD} -w ${PWD} \
		$(NIX_IMAGE) nix --print-build-logs --option cores 8 --option max-jobs 8 build --file nix/ --extra-experimental-features nix-command
	mkdir -p bin
	cp -r result/bin bin/static

crio.conf: bin/crio ## Build the CRI-O configuration.
	./bin/crio -d "" --config="" $(CONF_OVERRIDES) config > crio.conf

# the approach here, rather than this target depending on the build targets
# directly, is such that each target should try to build regardless if it
# fails. And return a non-zero exit if _any_ target fails.
.PHONY: local-cross
local-cross: ## Build the cross compilation targets.
	@$(MAKE) --keep-going $(CROSS_BUILD_TARGETS)

bin/crio.cross.%:
	@echo "==> make $@"; \
	TARGET="$*"; \
	GOOS="$${TARGET%%.*}" \
	GOARCH="$${TARGET##*.}" \
	$(GO_BUILD) $(GO_LDFLAGS) -tags "containers_image_openpgp btrfs_noversion" -o "$@" ./cmd/crio

.PHONY: bin/metrics-exporter
bin/metrics-exporter: ## Build the metrics exporter.
	$(GO_BUILD) -o $@ \
		-ldflags '-linkmode external -extldflags "-static -lm"' \
		-tags netgo \
		./contrib/metrics-exporter

.PHONY: metrics-exporter
metrics-exporter: bin/metrics-exporter ## Build the metrics exporter container.
	$(CONTAINER_RUNTIME) build . \
		-f contrib/metrics-exporter/Containerfile \
		-t quay.io/crio/metrics-exporter:latest

.PHONY: install
install: install.bin install.man install.completions install.systemd install.config ## Install the project locally.

.PHONY: install.bin-nobuild
install.bin-nobuild: ## Install the binaries.
	install ${SELINUXOPT} -D -m 755 bin/crio $(BINDIR)/crio
	install ${SELINUXOPT} -D -m 755 bin/pinns $(BINDIR)/pinns

.PHONY: install.bin
install.bin: binaries install.bin-nobuild ## Build and install the binaries.

.PHONY: install.man-nobuild
install.man-nobuild: ## Install the man pages.
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man5
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man8
	install ${SELINUXOPT} -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install ${SELINUXOPT} -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8

.PHONY: install.man
install.man: $(MANPAGES) install.man-nobuild ## Build and install the man pages.

.PHONY: install.config-nobuild
install.config-nobuild: ## Install the configuration files.
	install ${SELINUXOPT} -d $(DATAROOTDIR)/oci/hooks.d
	install ${SELINUXOPT} -d $(ETCDIR_CRIO)/crio.conf.d
	install ${SELINUXOPT} -D -m 644 crio.conf $(ETCDIR_CRIO)/crio.conf
	install ${SELINUXOPT} -D -m 644 crio-umount.conf $(OCIUMOUNTINSTALLDIR)/crio-umount.conf
	install ${SELINUXOPT} -D -m 644 crictl.yaml $(CRICTL_CONFIG_DIR)

.PHONY: install.config
install.config: crio.conf install.config-nobuild ## Build and install the configuration files.

.PHONY: install.completions
install.completions: ## Install the completions.
	install ${SELINUXOPT} -d -m 755 ${BASHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${FISHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${ZSHINSTALLDIR}
	install ${SELINUXOPT} -D -m 644 -t ${BASHINSTALLDIR} completions/bash/crio
	install ${SELINUXOPT} -D -m 644 -t ${FISHINSTALLDIR} completions/fish/crio.fish
	install ${SELINUXOPT} -D -m 644 -t ${ZSHINSTALLDIR}  completions/zsh/_crio

.PHONY: install.systemd
install.systemd: ## Install the systemd unit files.
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio.service $(PREFIX)/lib/systemd/system/crio.service
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio-wipe.service $(PREFIX)/lib/systemd/system/crio-wipe.service

.PHONY: uninstall
uninstall: ## Uninstall all files.
	rm -f $(BINDIR)/crio
	rm -f $(BINDIR)/pinns
	for i in $(filter %.5,$(MANPAGES)); do \
		rm -f $(MANDIR)/man5/$$(basename $${i}); \
	done
	for i in $(filter %.8,$(MANPAGES)); do \
		rm -f $(MANDIR)/man8/$$(basename $${i}); \
	done
	rm -f ${BASHINSTALLDIR}/crio
	rm -f ${FISHINSTALLDIR}/crio.fish
	rm -f ${ZSHINSTALLDIR}/_crio
	rm -f $(PREFIX)/lib/systemd/system/crio-wipe.service
	rm -f $(PREFIX)/lib/systemd/system/crio.service
	rm -f $(PREFIX)/lib/systemd/system/cri-o.service
	rm -rf $(DATAROOTDIR)/oci/hooks.d
	rm -f $(ETCDIR_CRIO)/crio.conf
	rm -rf $(ETCDIR_CRIO)/crio.conf.d
	rm -f $(OCIUMOUNTINSTALLDIR)/crio-umount.conf
	rm -f $(CRICTL_CONFIG_DIR)/crictl.yaml

##@ Verify targets:

.PHONY: lint
lint:  ${GOLANGCI_LINT} ## Run the golang linter, see also: .github/workflows/verify.yml.
	${GOLANGCI_LINT} version
	${GOLANGCI_LINT} linters
	GL_DEBUG=gocritic ${GOLANGCI_LINT} run

.PHONY: check-log-lines
check-log-lines: ## Verify that all log lines start with a capitalized letter.
	./hack/log-capitalized.sh
	./hack/tree_status.sh

.PHONY: check-config-template
check-config-template: ## Validate that the config template is correct.
	./hack/validate-config.sh

.PHONY: shellfiles
shellfiles: ${SHFMT}
	$(eval SHELLFILES=$(shell ${SHFMT} -f . | grep -v vendor/ | grep -v hack/lib | grep -v hack/build-rpms.sh | grep -v .bats))

.PHONY: shfmt
shfmt: shellfiles ## Run shfmt on all shell files.
	${SHFMT} -ln bash -w -i 4 -d ${SHELLFILES}
	${SHFMT} -ln bats -w -sr -d $(BATS_FILES)

.PHONY: shellcheck
shellcheck: shellfiles ${SHELLCHECK} ## Run shellcheck on all shell files.
	${SHELLCHECK} \
		-P scripts \
		-P test \
		-x \
		${SHELLFILES} ${BATS_FILES}

.PHONY: check-nri-bats-tests
check-nri-bats-tests: test/nri/nri.test ## Run the bats NRI tests.
	./hack/check-nri-bats-tests.sh

.PHONY: check-vendor
check-vendor: vendor ## Check the vendored golang dependencies.
	./hack/tree_status.sh

.PHONY: testunit
testunit: ${GINKGO} ## Run the unit tests.
	rm -rf ${COVERAGE_PATH} && mkdir -p ${COVERAGE_PATH}
	${BUILD_BIN_PATH}/ginkgo run \
		${TESTFLAGS} \
		-r \
		--skip-package $(GINKGO_SKIP_PACKAGES) \
		--trace \
		--cover \
		--covermode atomic \
		--output-dir ${COVERAGE_PATH} \
		--junit-report junit.xml \
		--coverprofile coverprofile \
		--tags "test $(BUILDTAGS)" \
		$(GO_MOD_VENDOR) \
		--succinct
	$(GO) tool cover -html=${COVERAGE_PATH}/coverprofile -o ${COVERAGE_PATH}/coverage.html

.PHONY: localintegration
localintegration: clean binaries test-binaries ## Run the local integration tests.
	./test/test_runner.sh ${TESTFLAGS}

.PHONY: verify-dependencies
verify-dependencies: ${ZEITGEIST} ## Verify the local dependencies.
	${BUILD_BIN_PATH}/zeitgeist validate --local-only --base-path . --config dependencies.yaml

.PHONY: verify-gosec
verify-gosec: ${GOSEC} ## Run gosec on the project.
	${BUILD_BIN_PATH}/gosec -exclude-dir=test -exclude-dir=_output -severity high -confidence high -exclude G304,G108 ./...

.PHONY: verify-govulncheck
verify-govulncheck: ## Check common vulnerabilities.
	./hack/govulncheck.sh

.PHONY: verify-mdtoc
verify-mdtoc: ${MDTOC} ## Verify the table of contents for the docs.
	git grep --name-only '<!-- toc -->' | grep -v Makefile | xargs ${MDTOC} -i -m=5
	./hack/tree_status.sh

.PHONY: verify-prettier
verify-prettier: prettier ## Run prettier on the project.
	./hack/tree_status.sh

##@ Utility targets:

.PHONY: clean
clean: ## Clean the repository.
	rm -rf _output
	rm -f docs/*.5 docs/*.8
	rm -f crio.conf
	rm -fr test/testdata/redis-image
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -rf bin/
	$(MAKE) -C pinns clean
	rm -f test/copyimg/copyimg
	rm -f test/checkseccomp/checkseccomp
	rm -f test/checkcriu/checkcriu
	rm -f test/nri/nri.test
	rm -rf ${BUILD_BIN_PATH}

.PHONY: nixpkgs
nixpkgs: ## Update the NIX package dependencies.
	@nix run -f channel:nixpkgs-unstable nix-prefetch-git -- \
		--no-deepClone https://github.com/nixos/nixpkgs > nix/nixpkgs.json

.PHONY: vendor
vendor: export GOSUMDB :=
vendor: ## Update the vendored dependencies.
	$(GO) mod tidy
	$(GO) mod vendor
	$(GO) mod verify

.PHONY: testunit-bin
testunit-bin: ## Build the unit test binaries.
	mkdir -p ${TESTBIN_PATH}
	for PACKAGE in `$(GO) list ./...`; do \
		go test $$PACKAGE \
			--tags "test $(BUILDTAGS)" \
			--gcflags '-N' -c -o ${TESTBIN_PATH}/$$(basename $$PACKAGE) ;\
	done

.PHONY: mockgen
mockgen: ## Regenerate all mocks.
mockgen: \
	mock-cmdrunner \
	mock-containerstorage \
	mock-containereventserver \
	mock-criostorage \
	mock-lib-config \
	mock-oci \
	mock-image-types \
	mock-ocicni-types \
	mock-seccompociartifact-types \
	mock-ociartifact-types \
	mock-systemd

.PHONY: mock-containereventserver
mock-containereventserver: ${MOCKGEN}
	${MOCKGEN} \
		-package containereventservermock \
		-destination ${MOCK_PATH}/containereventserver/containereventserver.go \
		k8s.io/cri-api/pkg/apis/runtime/v1 RuntimeService_GetContainerEventsServer

.PHONY: mock-containerstorage
mock-containerstorage: ${MOCKGEN}
	${MOCKGEN} \
		-package containerstoragemock \
		-destination ${MOCK_PATH}/containerstorage/containerstorage.go \
		github.com/containers/storage Store

.PHONY: mock-cmdrunner
mock-cmdrunner: ${MOCKGEN}
	${MOCKGEN} \
		-package cmdrunnermock \
		-destination ${MOCK_PATH}/cmdrunner/cmdrunner.go \
		github.com/cri-o/cri-o/utils/cmdrunner CommandRunner

.PHONY: mock-criostorage
mock-criostorage: ${MOCKGEN}
	${MOCKGEN} \
		-package criostoragemock \
		-destination ${MOCK_PATH}/criostorage/criostorage.go \
		github.com/cri-o/cri-o/internal/storage ImageServer,RuntimeServer,StorageTransport

.PHONY: mock-lib-config
mock-lib-config: ${MOCKGEN}
	${MOCKGEN} \
		-package libconfigmock \
		-destination ${MOCK_PATH}/lib/lib.go \
		github.com/cri-o/cri-o/pkg/config Iface

.PHONY: mock-oci
mock-oci: ${MOCKGEN}
	${MOCKGEN} \
		-package ocimock \
		-destination ${MOCK_PATH}/oci/oci.go \
		github.com/cri-o/cri-o/internal/oci RuntimeImpl

.PHONY: mock-image-types
mock-image-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package imagetypesmock \
		-destination ${MOCK_PATH}/containers/image/v5/types.go \
		github.com/containers/image/v5/types ImageCloser

.PHONY: mock-ocicni-types
mock-ocicni-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package ocicnitypesmock \
		-destination ${MOCK_PATH}/ocicni/types.go \
		github.com/cri-o/ocicni/pkg/ocicni CNIPlugin

.PHONY: mock-seccompociartifact-types
mock-seccompociartifact-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package seccompociartifactmock \
		-destination ${MOCK_PATH}/seccompociartifact/seccompociartifact.go \
		github.com/cri-o/cri-o/internal/config/seccomp/seccompociartifact Impl

.PHONY: mock-ociartifact-types
mock-ociartifact-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package ociartifactmock \
		-destination ${MOCK_PATH}/ociartifact/ociartifact.go \
		github.com/cri-o/cri-o/internal/config/ociartifact Impl

.PHONY: mock-systemd
mock-systemd: ${MOCKGEN}
	${MOCKGEN} \
		-package systemdmock \
		-destination ${MOCK_PATH}/systemd/systemd.go \
		github.com/cri-o/cri-o/internal/watchdog Systemd

docs/%.5: docs/%.5.md ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs/%.8: docs/%.8.md ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

.PHONY: completions-generation
completions-generation: ## Generate the command line shell completions.
	bin/crio complete bash > completions/bash/crio
	bin/crio complete fish > completions/fish/crio.fish
	bin/crio complete zsh  > completions/zsh/_crio

.PHONY: docs
docs: $(MANPAGES) ## Build the man pages.

.PHONY: docs-generation
docs-generation: ## Generate the documentation.
	bin/crio -d "" --config="" md  > docs/crio.8.md
	bin/crio -d "" --config="" man > docs/crio.8

.PHONY: prettier
prettier: ## Prettify supported files.
	$(CONTAINER_RUNTIME) run -it --privileged -v ${PWD}:/w -w /w --entrypoint bash node:latest -c \
		'npm install -g prettier && prettier -w .'

.PHONY: docs-validation
docs-validation: ## Validate the documentation.
	$(GO_RUN) -tags "$(BUILDTAGS)" ./test/docs-validation

##@ CI targets:

.PHONY: release
release: ## Run the release script.
	${GO_RUN} ./scripts/release

.PHONY: tag-reconciler
tag-reconciler: ## Run the release tag reconciler script.
	${GO_RUN} ./scripts/tag-reconciler

.PHONY: release-notes
release-notes: ${RELEASE_NOTES} ## Run the release notes tool.
	${GO_RUN} ./scripts/release-notes \
		--output-path ${BUILD_PATH}/release-notes

.PHONY: dependencies
dependencies: ${GO_MOD_OUTDATED} ## Run the golang dependency report.
	${GO_RUN} ./scripts/dependencies \
		--output-path ${BUILD_PATH}/dependencies

.PHONY: release-branch-forward
release-branch-forward: ## Run the release branch fast forward script.
	$(GO_RUN) ./scripts/release-branch-forward

.PHONY: upload-artifacts
upload-artifacts: ## Upload the built artifacts.
	./scripts/upload-artifacts
