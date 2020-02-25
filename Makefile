GO ?= go

export GOPROXY=https://proxy.golang.org
export GOSUMDB=https://sum.golang.org

GO_MOD_VENDOR := --mod=vendor
GO_BUILD := GO111MODULE=on $(GO) build $(GO_MOD_VENDOR)
GO_RUN := GO111MODULE=on $(GO) run $(GO_MOD_VENDOR)

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
			 $(shell hack/selinux_tag.sh)
CRICTL_CONFIG_DIR=${DESTDIR}/etc
CONTAINER_RUNTIME ?= podman
BUILD_PATH := $(shell pwd)/build
BUILD_BIN_PATH := ${BUILD_PATH}/bin
COVERAGE_PATH := ${BUILD_PATH}/coverage
JUNIT_PATH := ${BUILD_PATH}/junit
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
GIT_VALIDATION := ${BUILD_BIN_PATH}/git-validation
RELEASE_TOOL := ${BUILD_BIN_PATH}/release-tool
GOLANGCI_LINT := ${BUILD_BIN_PATH}/golangci-lint

ifeq ($(shell bash -c '[[ `command -v git` && `git rev-parse --git-dir 2>/dev/null` ]] && echo true'), true)
	COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
	GIT_TREE_STATE := $(if $(shell git status --porcelain --untracked-files=no),dirty,clean)
	GIT_MERGE_BASE := $(shell git merge-base origin/master $(shell git rev-parse --abbrev-ref HEAD))
else
	COMMIT_NO := unknown
	GIT_TREE_STATE := unknown
	GIT_MERGE_BASE := HEAD^
endif

# pass crio CLI options to generate custom crio.conf build time
CONF_OVERRIDES ?=

CROSS_BUILD_TARGETS := \
	bin/crio.cross.windows.amd64 \
	bin/crio.cross.darwin.amd64 \
	bin/crio.cross.linux.amd64

# If GOPATH not specified, use one in the local directory
ifeq ($(GOPATH),)
export GOPATH := $(CURDIR)/_output
unexport GOBIN
endif
GOPKGDIR := $(GOPATH)/src/$(PROJECT)
GOPKGBASEDIR := $(shell dirname "$(GOPKGDIR)")
GO_FILES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

# Update VPATH so make finds .gopathok
VPATH := $(VPATH):$(GOPATH)
SHRINKFLAGS := -s -w
VERSION := $(shell $(GO_RUN) ./scripts/latest_version.go)
DEFAULTS_PATH := ""

BASE_LDFLAGS = ${SHRINKFLAGS} \
	-X ${PROJECT}/internal/pkg/criocli.DefaultsPath=${DEFAULTS_PATH} \
	-X ${PROJECT}/internal/version.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
	-X ${PROJECT}/internal/version.gitCommit=${COMMIT_NO} \
	-X ${PROJECT}/internal/version.gitTreeState=${GIT_TREE_STATE} \
	-X ${PROJECT}/internal/version.version=${VERSION}

LDFLAGS = -ldflags '${BASE_LDFLAGS} ${EXTRA_LDFLAGS}'

TESTIMAGE_VERSION := master-1.2.0
TESTIMAGE_REGISTRY := quay.io/crio
TESTIMAGE_SCRIPT := scripts/build-test-image -r $(TESTIMAGE_REGISTRY) -v $(TESTIMAGE_VERSION)
TESTIMAGE_NAME ?= $(shell $(TESTIMAGE_SCRIPT) -d)

TESTIMAGE_NIX ?= $(TESTIMAGE_REGISTRY)/nix:1.2.0

all: binaries crio.conf docs

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations"
	@echo " * 'binaries' - Build crio and pinns"
	@echo " * 'release-note' - Generate release note"
	@echo " * 'integration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"

# Dummy target for marking pattern rules phony
.explicit_phony:

.gopathok:
ifeq ("$(wildcard $(GOPKGDIR))","")
	mkdir -p "$(GOPKGBASEDIR)"
	ln -s "$(CURDIR)" "$(GOPKGDIR)"
endif
	touch "$(GOPATH)/.gopathok"

lint: .gopathok ${GOLANGCI_LINT}
	${GOLANGCI_LINT} version
	${GOLANGCI_LINT} linters
	${GOLANGCI_LINT} run

bin/pinns:
	$(MAKE) -C pinns

test/copyimg/copyimg: $(GO_FILES) .gopathok
	$(GO_BUILD) $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/copyimg

test/checkseccomp/checkseccomp: $(GO_FILES) .gopathok
	$(GO_BUILD) $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/checkseccomp

bin/crio: $(GO_FILES) .gopathok
	$(GO_BUILD) $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/cmd/crio

bin/crio-status: $(GO_FILES) .gopathok
	$(GO_BUILD) $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/cmd/crio-status

build-static:
	$(CONTAINER_RUNTIME) run --rm -it -v $(shell pwd):/cri-o $(TESTIMAGE_NIX) sh -c \
		"nix build -f cri-o/nix --argstr revision $(COMMIT_NO) && \
		mkdir -p cri-o/bin && \
		cp result-*bin/bin/crio-* cri-o/bin"

release-bundle: clean bin/pinns build-static docs crio.conf bundle

crio.conf: bin/crio
	./bin/crio --config="" $(CONF_OVERRIDES) config  > crio.conf

release-note: ${RELEASE_TOOL}
	${RELEASE_TOOL} -n $(release)

clean:
ifneq ($(GOPATH),)
	rm -f "$(GOPATH)/.gopathok"
endif
	rm -rf _output
	rm -f docs/*.5 docs/*.8
	rm -fr test/testdata/redis-image
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -f bin/crio
	rm -f bin/crio.cross.*
	$(MAKE) -C pinns clean
	rm -f test/copyimg/copyimg
	rm -f test/checkseccomp/checkseccomp
	rm -rf ${BUILD_BIN_PATH}

# the approach here, rather than this target depending on the build targets
# directly, is such that each target should try to build regardless if it
# fails. And return a non-zero exit if _any_ target fails.
local-cross:
	@$(MAKE) --keep-going $(CROSS_BUILD_TARGETS)

bin/crio.cross.%: .gopathok .explicit_phony
	@echo "==> make $@"; \
	TARGET="$*"; \
	GOOS="$${TARGET%%.*}" \
	GOARCH="$${TARGET##*.}" \
	$(GO_BUILD) $(LDFLAGS) -tags "containers_image_openpgp btrfs_noversion" -o "$@" $(PROJECT)/cmd/crio

local-image:
	$(TESTIMAGE_SCRIPT)

test-images:
	$(TESTIMAGE_SCRIPT) -g 1.14 -a amd64
	$(TESTIMAGE_SCRIPT) -g 1.14 -a 386
	$(TESTIMAGE_SCRIPT) -g 1.12 -a amd64

test-image-nix:
	time $(CONTAINER_RUNTIME) build -t $(TESTIMAGE_NIX) \
		--build-arg COMMIT=$(COMMIT_NO) -f Dockerfile-nix .

dbuild:
	$(CONTAINER_RUNTIME) run --rm --name=${CRIO_INSTANCE} --privileged \
		-v $(shell pwd):/go/src/${PROJECT} -w /go/src/${PROJECT} \
		$(TESTIMAGE_NAME) make

integration: ${GINKGO}
	$(CONTAINER_RUNTIME) run \
		-e CI=true \
		-e CRIO_BINARY \
		-e JOBS \
		-e RUN_CRITEST \
		-e STORAGE_OPTIONS="-s=vfs" \
		-e TESTFLAGS \
		-e TEST_USERNS \
		-it --privileged --rm \
		-v $(shell pwd):/go/src/${PROJECT} \
		-v ${GINKGO}:/usr/bin/ginkgo \
		-w /go/src/${PROJECT} \
		--sysctl net.ipv6.conf.all.disable_ipv6=0 \
		$(TESTIMAGE_NAME) \
		make localintegration

define go-build
	$(shell cd `pwd` && $(GO_BUILD) -o $(BUILD_BIN_PATH)/$(shell basename $(1)) $(1))
	@echo > /dev/null
endef

${GO_MD2MAN}:
	$(call go-build,./vendor/github.com/cpuguy83/go-md2man)

${GINKGO}:
	$(call go-build,./vendor/github.com/onsi/ginkgo/ginkgo)

${MOCKGEN}:
	$(call go-build,./vendor/github.com/golang/mock/mockgen)

${GIT_VALIDATION}:
	$(call go-build,./vendor/github.com/vbatts/git-validation)

${RELEASE_TOOL}:
	$(call go-build,./vendor/github.com/containerd/project/cmd/release-tool)

${GOLANGCI_LINT}:
	export \
		VERSION=v1.23.6 \
		URL=https://raw.githubusercontent.com/golangci/golangci-lint \
		BINDIR=${BUILD_BIN_PATH} && \
	curl -sfL $$URL/$$VERSION/install.sh | sh -s $$VERSION

vendor:
	export GO111MODULE=on \
		$(GO) mod tidy && \
		$(GO) mod vendor && \
		$(GO) mod verify

testunit: ${GINKGO}
	rm -rf ${COVERAGE_PATH} && mkdir -p ${COVERAGE_PATH}
	rm -rf ${JUNIT_PATH} && mkdir -p ${JUNIT_PATH}
	${BUILD_BIN_PATH}/ginkgo \
		${TESTFLAGS} \
		-r \
		--trace \
		--cover \
		--covermode atomic \
		--outputdir ${COVERAGE_PATH} \
		--coverprofile coverprofile \
		--tags "test $(BUILDTAGS)" \
		$(GO_MOD_VENDOR) \
		--succinct
	$(GO) tool cover -html=${COVERAGE_PATH}/coverprofile -o ${COVERAGE_PATH}/coverage.html
	$(GO) tool cover -func=${COVERAGE_PATH}/coverprofile | sed -n 's/\(total:\).*\([0-9][0-9].[0-9]\)/\1 \2/p'
	find . -name '*_junit.xml' -exec mv -t ${JUNIT_PATH} {} +

testunit-bin:
	mkdir -p ${TESTBIN_PATH}
	for PACKAGE in `$(GO) list ./...`; do \
		go test $$PACKAGE \
			--tags "test $(BUILDTAGS)" \
			--gcflags '-N' -c -o ${TESTBIN_PATH}/$$(basename $$PACKAGE) ;\
	done

mockgen: \
	mock-containerstorage \
	mock-criostorage \
	mock-lib-config \
	mock-oci \
	mock-sandbox \
	mock-image-types \
	mock-ocicni-types

mock-containerstorage: ${MOCKGEN}
	${MOCKGEN} \
		-package containerstoragemock \
		-destination ${MOCK_PATH}/containerstorage/containerstorage.go \
		github.com/containers/storage Store

mock-criostorage: ${MOCKGEN}
	${MOCKGEN} \
		-package criostoragemock \
		-destination ${MOCK_PATH}/criostorage/criostorage.go \
		github.com/cri-o/cri-o/internal/pkg/storage ImageServer,RuntimeServer

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

mock-sandbox: ${MOCKGEN}
	${MOCKGEN} \
		-package sandboxmock \
		-destination ${MOCK_PATH}/sandbox/sandbox.go \
		github.com/cri-o/cri-o/internal/lib/sandbox NamespaceIface

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

binaries: bin/crio bin/crio-status bin/pinns
test-binaries: test/copyimg/copyimg test/checkseccomp/checkseccomp

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

docs/%.5: docs/%.5.md .gopathok ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs/%.8: docs/%.8.md .gopathok ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

completions:
	bin/crio complete bash > completions/bash/crio
	bin/crio complete fish > completions/fish/crio.fish
	bin/crio complete zsh  > completions/zsh/_crio
	bin/crio-status complete bash > completions/bash/crio-status
	bin/crio-status complete fish > completions/fish/crio-status.fish
	bin/crio-status complete zsh  > completions/zsh/_crio-status

docs: $(MANPAGES)

docs-generation:
	bin/crio-status md  > docs/crio-status.8.md
	bin/crio-status man > docs/crio-status.8
	bin/crio --config="" md  > docs/crio.8.md
	bin/crio --config="" man > docs/crio.8

bundle:
	bundle/build

install: .gopathok install.bin install.man install.completions install.systemd install.config

install.bin: binaries
	install ${SELINUXOPT} -D -m 755 bin/crio $(BINDIR)/crio
	install ${SELINUXOPT} -D -m 755 bin/crio-status $(BINDIR)/crio-status
	install ${SELINUXOPT} -D -m 755 bin/pinns $(BINDIR)/pinns

install.man: $(MANPAGES)
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man5
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man8
	install ${SELINUXOPT} -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install ${SELINUXOPT} -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8

install.config: crio.conf
	install ${SELINUXOPT} -d $(DATAROOTDIR)/oci/hooks.d
	install ${SELINUXOPT} -D -m 644 crio.conf $(ETCDIR_CRIO)/crio.conf
	install ${SELINUXOPT} -D -m 644 crio-umount.conf $(OCIUMOUNTINSTALLDIR)/crio-umount.conf
	install ${SELINUXOPT} -D -m 644 crictl.yaml $(CRICTL_CONFIG_DIR)

install.completions:
	install ${SELINUXOPT} -d -m 755 ${BASHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${FISHINSTALLDIR}
	install ${SELINUXOPT} -d -m 755 ${ZSHINSTALLDIR}
	install ${SELINUXOPT} -D -m 644 -t ${BASHINSTALLDIR} completions/bash/crio
	install ${SELINUXOPT} -D -m 644 -t ${FISHINSTALLDIR} completions/fish/crio.fish
	install ${SELINUXOPT} -D -m 644 -t ${ZSHINSTALLDIR}  completions/zsh/_crio
	install ${SELINUXOPT} -D -m 644 -t ${BASHINSTALLDIR} completions/bash/crio-status
	install ${SELINUXOPT} -D -m 644 -t ${FISHINSTALLDIR} completions/fish/crio-status.fish
	install ${SELINUXOPT} -D -m 644 -t ${ZSHINSTALLDIR}  completions/zsh/_crio-status

install.systemd:
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio.service $(PREFIX)/lib/systemd/system/crio.service
	ln -sf crio.service $(PREFIX)/lib/systemd/system/cri-o.service
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio-shutdown.service $(PREFIX)/lib/systemd/system/crio-shutdown.service
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio-wipe.service $(PREFIX)/lib/systemd/system/crio-wipe.service

uninstall:
	rm -f $(BINDIR)/crio
	rm -f $(BINDIR)/crio-status
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
	rm -f ${BASHINSTALLDIR}/crio-status
	rm -f ${FISHINSTALLDIR}/crio-status.fish
	rm -f ${ZSHINSTALLDIR}/_crio-status

git-validation: .gopathok ${GIT_VALIDATION}
	GIT_CHECK_EXCLUDE="vendor" \
		${GIT_VALIDATION} -v -run DCO,short-subject,dangling-whitespace \
			-range ${GIT_MERGE_BASE}..HEAD

docs-validation:
	$(GO_RUN) -tags "$(BUILDTAGS)" ./test/docs-validation

.PHONY: \
	.explicit_phony \
	git-validation \
	binaries \
	bundle \
	build-static \
	clean \
	completions \
	default \
	docs \
	docs-validation \
	help \
	install \
	lint \
	local-cross \
	release-bundle \
	testunit \
	testunit-bin \
	test-images \
	test-image-nix \
	uninstall \
	vendor \
	bin/pinns
