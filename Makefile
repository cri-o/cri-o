GO ?= go

export GOPROXY=https://proxy.golang.org
export GOSUMDB=https://sum.golang.org

TRIMPATH ?= -trimpath
GO_ARCH=$(shell $(GO) env GOARCH)
GO_MAJOR_VERSION = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f1)
GO_MINOR_VERSION = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
GO_GT_1_17 := $(shell [ $(GO_MAJOR_VERSION) -ge 1 -a $(GO_MINOR_VERSION) -ge 17 ] && echo true)
GO_FLAGS ?=
ifeq ($(GO_GT_1_17),true)
ifeq ($(GO_ARCH),386)
GO_FLAGS += -buildvcs=false
endif
endif

GO_BUILD ?= $(GO) build $(GO_FLAGS) $(TRIMPATH)
GO_RUN ?= $(GO) run
NIX_IMAGE ?= nixos/nix:2.3.16

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
			 $(shell hack/libdm_installed.sh) \
			 $(shell hack/libdm_no_deferred_remove_tag.sh) \
			 $(shell hack/openpgp_tag.sh) \
			 $(shell hack/seccomp_tag.sh) \
			 $(shell hack/selinux_tag.sh) \
			 $(shell hack/libsubid_tag.sh)
CRICTL_CONFIG_DIR=${DESTDIR}/etc
CONTAINER_RUNTIME ?= podman
BUILD_PATH := $(shell pwd)/build
BUILD_BIN_PATH := ${BUILD_PATH}/bin
COVERAGE_PATH := ${BUILD_PATH}/coverage
TESTBIN_PATH := ${BUILD_PATH}/test
MOCK_PATH := ${PWD}/test/mocks

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
FISHINSTALLDIR=${PREFIX}/share/fish/completions
ZSHINSTALLDIR=${PREFIX}/share/zsh/site-functions
OCIUMOUNTINSTALLDIR=$(PREFIX)/share/oci-umount/oci-umount.d

SELINUXOPT ?= $(shell selinuxenabled 2>/dev/null && echo -Z)

SOURCE_DATE_EPOCH ?= $(shell date +%s)

GO_MD2MAN ?= ${BUILD_BIN_PATH}/go-md2man
GINKGO := ${BUILD_BIN_PATH}/ginkgo
MOCKGEN := ${BUILD_BIN_PATH}/mockgen
MOCKGEN_VERSION := 1.6.0
GOLANGCI_LINT := ${BUILD_BIN_PATH}/golangci-lint
GOLANGCI_LINT_VERSION := v1.55.2
GO_MOD_OUTDATED := ${BUILD_BIN_PATH}/go-mod-outdated
GO_MOD_OUTDATED_VERSION := 0.9.0
GOSEC := ${BUILD_BIN_PATH}/gosec
GOSEC_VERSION := 2.18.2
RELEASE_NOTES := ${BUILD_BIN_PATH}/release-notes
ZEITGEIST := ${BUILD_BIN_PATH}/zeitgeist
ZEITGEIST_VERSION := v0.4.1
RELEASE_NOTES_VERSION := v0.16.4
SHFMT := ${BUILD_BIN_PATH}/shfmt
SHFMT_VERSION := v3.7.0
SHELLCHECK := ${BUILD_BIN_PATH}/shellcheck
SHELLCHECK_VERSION := v0.9.0
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

all: binaries crio.conf docs

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations"
	@echo " * 'binaries' - Build crio and pinns"
	@echo " * 'release-note' - Generate release note"
	@echo " * 'localintegration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'shfmt' - shell format check and apply diff"
	@echo " * 'shellcheck' - Execute the shellcheck linter"

# Dummy target for marking pattern rules phony
.explicit_phony:

# See also: .github/workflows/verify.yml.
lint:  ${GOLANGCI_LINT}
	${GOLANGCI_LINT} version
	${GOLANGCI_LINT} linters
	GL_DEBUG=gocritic ${GOLANGCI_LINT} run

check-log-lines:
	./hack/log-capitalized.sh
	./hack/tree_status.sh

check-config-template:
	./hack/validate-config.sh

shellfiles: ${SHFMT}
	$(eval SHELLFILES=$(shell ${SHFMT} -f . | grep -v vendor/ | grep -v hack/lib | grep -v hack/build-rpms.sh | grep -v .bats))

shfmt: shellfiles
	${SHFMT} -ln bash -w -i 4 -d ${SHELLFILES}
	${SHFMT} -ln bats -w -sr -d $(BATS_FILES)

shellcheck: shellfiles ${SHELLCHECK}
	${SHELLCHECK} \
		-P scripts \
		-P test \
		-x \
		${SHELLFILES} ${BATS_FILES}

check-nri-bats-tests: test/nri/nri.test
	./hack/check-nri-bats-tests.sh

bin/pinns:
	$(MAKE) -C pinns

test/copyimg/copyimg: $(GO_FILES)
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/copyimg

test/checkseccomp/checkseccomp: $(GO_FILES)
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/checkseccomp

test/checkcriu/checkcriu: $(GO_FILES)
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/checkcriu

test/nri/nri.test: $(wildcard test/nri/*.go)
	$(GO) test --tags "test $(BUILDTAGS)" -c $(PROJECT)/test/nri -o $@

bin/crio: $(GO_FILES)
	$(GO_BUILD) $(GCFLAGS) $(GO_LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/cmd/crio

build-static:
	$(CONTAINER_RUNTIME) run --network=host --rm --privileged -ti -v /:/mnt \
		$(NIX_IMAGE) cp -rfT /nix /mnt/nix
	$(CONTAINER_RUNTIME) run --network=host --rm --privileged -ti -v /nix:/nix -v ${PWD}:${PWD} -w ${PWD} \
		$(NIX_IMAGE) nix --print-build-logs --option cores 8 --option max-jobs 8 build --file nix/
	mkdir -p bin
	cp -r result/bin bin/static


crio.conf: bin/crio
	./bin/crio -d "" --config="" $(CONF_OVERRIDES) config > crio.conf

release:
	${GO_RUN} ./scripts/release

release-notes: ${RELEASE_NOTES}
	${GO_RUN} ./scripts/release-notes \
		--output-path ${BUILD_PATH}/release-notes

dependencies: ${GO_MOD_OUTDATED}
	${GO_RUN} ./scripts/dependencies \
		--output-path ${BUILD_PATH}/dependencies

clean:
	rm -rf _output
	rm -f docs/*.5 docs/*.8
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

# the approach here, rather than this target depending on the build targets
# directly, is such that each target should try to build regardless if it
# fails. And return a non-zero exit if _any_ target fails.
local-cross:
	@$(MAKE) --keep-going $(CROSS_BUILD_TARGETS)

bin/crio.cross.%:  .explicit_phony
	@echo "==> make $@"; \
	TARGET="$*"; \
	GOOS="$${TARGET%%.*}" \
	GOARCH="$${TARGET##*.}" \
	$(GO_BUILD) $(GO_LDFLAGS) -tags "containers_image_openpgp btrfs_noversion" -o "$@" $(PROJECT)/cmd/crio

nixpkgs:
	@nix run -f channel:nixpkgs-unstable nix-prefetch-git -- \
		--no-deepClone https://github.com/nixos/nixpkgs > nix/nixpkgs.json

define go-build
	$(shell cd `pwd` && $(GO_BUILD) -o $(BUILD_BIN_PATH)/$(shell basename $(1)) $(1))
	@echo > /dev/null
endef

$(BUILD_BIN_PATH):
	mkdir -p $(BUILD_BIN_PATH)

$(GO_MD2MAN):
	$(call go-build,./vendor/github.com/cpuguy83/go-md2man)

$(GINKGO):
	$(call go-build,./vendor/github.com/onsi/ginkgo/v2/ginkgo)

define curl_to
    curl -sSfL --retry 5 --retry-delay 3 "$(1)" -o $(2)
	chmod +x $(2)
endef

$(RELEASE_NOTES): $(BUILD_BIN_PATH)
	$(call curl_to,https://storage.googleapis.com/k8s-artifacts-sig-release/kubernetes/release/$(RELEASE_NOTES_VERSION)/release-notes-amd64-linux,$(RELEASE_NOTES))

$(SHFMT): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/mvdan/sh/releases/download/$(SHFMT_VERSION)/shfmt_$(SHFMT_VERSION)_linux_amd64,$(SHFMT))

$(ZEITGEIST): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/kubernetes-sigs/zeitgeist/releases/download/$(ZEITGEIST_VERSION)/zeitgeist_$(ZEITGEIST_VERSION:v%=%)_linux_amd64,$(BUILD_BIN_PATH)/zeitgeist)

$(MOCKGEN): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/golang/mock/releases/download/v$(MOCKGEN_VERSION)/mock_$(MOCKGEN_VERSION)_linux_$(GO_ARCH).tar.gz,$(BUILD_BIN_PATH)/mockgen.tar.gz)
	tar xf $(BUILD_BIN_PATH)/mockgen.tar.gz --strip-components=1 -C $(BUILD_BIN_PATH)

$(GO_MOD_OUTDATED): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/psampaz/go-mod-outdated/releases/download/v$(GO_MOD_OUTDATED_VERSION)/go-mod-outdated_$(GO_MOD_OUTDATED_VERSION)_Linux_x86_64.tar.gz,$(BUILD_BIN_PATH)/gmo.tar.gz)
	tar xf $(BUILD_BIN_PATH)/gmo.tar.gz -C $(BUILD_BIN_PATH)

$(GOSEC): $(BUILD_BIN_PATH)
	$(call curl_to,https://github.com/securego/gosec/releases/download/v$(GOSEC_VERSION)/gosec_$(GOSEC_VERSION)_linux_amd64.tar.gz,$(BUILD_BIN_PATH)/gosec.tar.gz)
	tar xf $(BUILD_BIN_PATH)/gosec.tar.gz -C $(BUILD_BIN_PATH)

$(GOLANGCI_LINT):
	export VERSION=$(GOLANGCI_LINT_VERSION) \
		URL=https://raw.githubusercontent.com/golangci/golangci-lint \
		BINDIR=${BUILD_BIN_PATH} && \
	curl -sSfL $$URL/$$VERSION/install.sh | sh -s $$VERSION

$(SHELLCHECK): $(BUILD_BIN_PATH)
	URL=https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).linux.x86_64.tar.xz \
	SHA256SUM=7087178d54de6652b404c306233264463cb9e7a9afeb259bb663cc4dbfd64149 && \
	curl -sSfL $$URL | tar xfJ - -C ${BUILD_BIN_PATH} --strip 1 shellcheck-$(SHELLCHECK_VERSION)/shellcheck && \
	sha256sum ${SHELLCHECK} | grep -q $$SHA256SUM

vendor: export GOSUMDB :=
vendor:
	$(GO) mod tidy
	$(GO) mod vendor
	$(GO) mod verify

check-vendor: vendor
	./hack/tree_status.sh

testunit: ${GINKGO}
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

testunit-bin:
	mkdir -p ${TESTBIN_PATH}
	for PACKAGE in `$(GO) list ./...`; do \
		go test $$PACKAGE \
			--tags "test $(BUILDTAGS)" \
			--gcflags '-N' -c -o ${TESTBIN_PATH}/$$(basename $$PACKAGE) ;\
	done

mockgen: \
	mock-cmdrunner \
	mock-containerstorage \
	mock-criostorage \
	mock-lib-config \
	mock-oci \
	mock-image-types \
	mock-ocicni-types

mock-containereventserver: ${MOCKGEN}
	${MOCKGEN} \
		-package containereventservermock \
		-destination ${MOCK_PATH}/containereventserver/containereventserver.go \
		k8s.io/cri-api/pkg/apis/runtime/v1 RuntimeService_GetContainerEventsServer

mock-containerstorage: ${MOCKGEN}
	${MOCKGEN} \
		-package containerstoragemock \
		-destination ${MOCK_PATH}/containerstorage/containerstorage.go \
		github.com/containers/storage Store

mock-cmdrunner: ${MOCKGEN}
	${MOCKGEN} \
		-package cmdrunnermock \
		-destination ${MOCK_PATH}/cmdrunner/cmdrunner.go \
		github.com/cri-o/cri-o/utils/cmdrunner CommandRunner

mock-criostorage: ${MOCKGEN}
	${MOCKGEN} \
		-package criostoragemock \
		-destination ${MOCK_PATH}/criostorage/criostorage.go \
		github.com/cri-o/cri-o/internal/storage ImageServer,RuntimeServer,StorageTransport

mock-lib-config: ${MOCKGEN}
	${MOCKGEN} \
		-package libconfigmock \
		-destination ${MOCK_PATH}/lib/lib.go \
		github.com/cri-o/cri-o/pkg/config Iface

mock-oci: ${MOCKGEN}
	${MOCKGEN} \
		-package ocimock \
		-destination ${MOCK_PATH}/oci/oci.go \
		github.com/cri-o/cri-o/internal/oci RuntimeImpl

mock-image-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package imagetypesmock \
		-destination ${MOCK_PATH}/containers/image/v5/types.go \
		github.com/containers/image/v5/types ImageCloser

mock-ocicni-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		-package ocicnitypesmock \
		-destination ${MOCK_PATH}/ocicni/types.go \
		github.com/cri-o/ocicni/pkg/ocicni CNIPlugin

codecov: SHELL := $(shell which bash)
codecov:
	bash <(curl -s https://codecov.io/bash) -f ${COVERAGE_PATH}/coverprofile

localintegration: clean binaries test-binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: bin/crio bin/pinns

test-binaries: test/copyimg/copyimg test/checkseccomp/checkseccomp test/checkcriu/checkcriu \
	test/nri/nri.test

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

docs/%.5: docs/%.5.md  ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs/%.8: docs/%.8.md  ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

completions-generation:
	bin/crio complete bash > completions/bash/crio
	bin/crio complete fish > completions/fish/crio.fish
	bin/crio complete zsh  > completions/zsh/_crio

docs: $(MANPAGES)

docs-generation:
	bin/crio -d "" --config="" md  > docs/crio.8.md
	bin/crio -d "" --config="" man > docs/crio.8

verify-dependencies: ${ZEITGEIST}
	${BUILD_BIN_PATH}/zeitgeist validate --local-only --base-path . --config dependencies.yaml

verify-gosec: ${GOSEC}
	${BUILD_BIN_PATH}/gosec -exclude-dir=test -exclude-dir=_output -severity high -confidence high -exclude G304,G108 ./...

verify-govulncheck:
	./hack/govulncheck.sh

install: install.bin install.man install.completions install.systemd install.config

install.bin-nobuild:
	install ${SELINUXOPT} -D -m 755 bin/crio $(BINDIR)/crio
	install ${SELINUXOPT} -D -m 755 bin/pinns $(BINDIR)/pinns

install.bin: binaries install.bin-nobuild

install.man-nobuild:
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man5
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man8
	install ${SELINUXOPT} -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install ${SELINUXOPT} -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8

install.man: $(MANPAGES) install.man-nobuild

install.config-nobuild:
	install ${SELINUXOPT} -d $(DATAROOTDIR)/oci/hooks.d
	install ${SELINUXOPT} -d $(ETCDIR_CRIO)/crio.conf.d
	install ${SELINUXOPT} -D -m 644 crio.conf $(ETCDIR_CRIO)/crio.conf
	install ${SELINUXOPT} -D -m 644 crio-umount.conf $(OCIUMOUNTINSTALLDIR)/crio-umount.conf
	install ${SELINUXOPT} -D -m 644 crictl.yaml $(CRICTL_CONFIG_DIR)

install.config: crio.conf install.config-nobuild

install.completions:
	install ${SELINUXOPT} -d -m 755 ${BASHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${FISHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${ZSHINSTALLDIR}
	install ${SELINUXOPT} -D -m 644 -t ${BASHINSTALLDIR} completions/bash/crio
	install ${SELINUXOPT} -D -m 644 -t ${FISHINSTALLDIR} completions/fish/crio.fish
	install ${SELINUXOPT} -D -m 644 -t ${ZSHINSTALLDIR}  completions/zsh/_crio

install.systemd:
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio.service $(PREFIX)/lib/systemd/system/crio.service
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio-wipe.service $(PREFIX)/lib/systemd/system/crio-wipe.service

uninstall:
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

docs-validation:
	$(GO_RUN) -tags "$(BUILDTAGS)" ./test/docs-validation

release-branch-forward:
	$(GO_RUN) ./scripts/release-branch-forward

upload-artifacts:
	./scripts/upload-artifacts

bin/metrics-exporter:
	$(GO_BUILD) -o $@ \
		-ldflags '-linkmode external -extldflags "-static -lm"' \
		-tags netgo \
		$(PROJECT)/contrib/metrics-exporter

metrics-exporter: bin/metrics-exporter
	$(CONTAINER_RUNTIME) build . \
		-f contrib/metrics-exporter/Containerfile \
		-t quay.io/crio/metrics-exporter:latest

.PHONY: \
	.explicit_phony \
	git-validation \
	binaries \
	build-static \
	clean \
	completions \
	config \
	default \
	docs \
	docs-validation \
	gosec \
	help \
	install \
	lint \
	local-cross \
	nixpkgs \
	shellfiles \
	shfmt \
	release-branch-forward \
	shellcheck \
	testunit \
	testunit-bin \
	test-images \
	uninstall \
	vendor \
	check-vendor \
	bin/pinns \
	dependencies \
	upload-artifacts \
	bin/metrics-exporter \
	metrics-exporter \
	release \
	check-log-lines \
	verify-dependencies
