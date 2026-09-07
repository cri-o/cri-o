.PHONY: check
check: check-licenses check-fmt test

.PHONY: check-licenses
check-licenses:
	go tool github.com/elastic/go-licenser -d .

.PHONY: update-licenses
update-licenses:
	go tool github.com/elastic/go-licenser .

_GOIMPORTS:=$(shell go tool golang.org/x/tools/cmd/goimports -l .)

.PHONY: check-fmt
check-fmt:
	@if [ -n "$(_GOIMPORTS)" ]; then printf "goimports differs: $(_GOIMPORTS) (run 'make fmt')\n" && exit 1; fi

.PHONY: fmt
fmt:
	go tool golang.org/x/tools/cmd/goimports -w .

.PHONY: test
test:
	go tool gotest.tools/gotestsum --format testname -- -race -v ./...
