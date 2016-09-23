EPOCH_TEST_COMMIT ?= 7fc874e05e74faa81e7c423b6514fc5c474c6b34

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'binaries' - Build ocid, conmon and ocic"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"

lint:
	@echo "checking lint"
	@./.tool/lint

conmon:
	make -C $@

ocid:
	go build -o ocid ./cmd/server/

ocic:
	go build -o ocic ./cmd/client/

clean:
	rm -f ocic ocid
	rm -f conmon/conmon.o conmon/conmon

binaries: ocid ocic conmon

.PHONY: .gitvalidation
# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make install.tools' target" && false)
ifeq ($(TRAVIS),true)
	git-validation -q -run DCO,short-subject
else
	git-validation -v -run DCO,short-subject -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: install.tools

install.tools: .install.gitvalidation .install.glide .install.glide-vc .install.gometalinter

.install.gitvalidation:
	go get github.com/vbatts/git-validation

.install.glide:
	go get github.com/Masterminds/glide

.install.glide-vc:
	go get github.com/sgotti/glide-vc

.install.gometalinter:
	go get github.com/alecthomas/gometalinter
	gometalinter --install

.PHONY: \
	binaries \
	conmon \
	ocid \
	ocic \
	clean \
	lint
