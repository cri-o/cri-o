EPOCH_TEST_COMMIT ?= 7fc874e05e74faa81e7c423b6514fc5c474c6b34

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'binaries' - Build ocid, conmon andocic"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'update-deps' - Update vendored dependencies"

lint:
	@echo "checking lint"
	@./.tool/lint

conmon:
	make -C $@

ocid:
	go build -o ocid ./cmd/server/main.go

ocic:
	go build -o ocic ./cmd/client/main.go

clean:
	rm -f ocic ocid
	rm -f conmon/conmon.o conmon/conmon

update-deps:
	@which glide > /dev/null 2>/dev/null || (echo "ERROR: glide not found." && false)
	glide update --strip-vcs --strip-vendor --update-vendored --delete
	glide-vc --only-code --no-tests
	# see http://sed.sourceforge.net/sed1line.txt
	find vendor -type f -exec sed -i -e :a -e '/^\n*$$/{$$d;N;ba' -e '}' "{}" \;

binaries: ocid ocic conmon

.PHONY: .gitvalidation
	#
# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make install.tools' target" && false)
ifeq ($(TRAVIS),true)
	git-validation -q -run DCO,short-subject,dangling-whitespace
else
	git-validation -v -run DCO,short-subject,dangling-whitespace -range $(EPOCH_TEST_COMMIT)..HEAD
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
	gometalinter --install --update

.PHONY: \
	binaries \
	clean \
	lint
