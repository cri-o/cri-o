.PHONY: git-vars
git-vars:
ifneq ($(wildcard .git),)
	$(eval COMMIT_NO :=$(shell git rev-parse HEAD 2> /dev/null || true))
	$(eval GIT_COMMIT := $(if $(shell git status --porcelain --untracked-files=no),"${COMMIT_NO}-dirty","${COMMIT_NO}"))
	$(eval GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
	$(eval GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g"))
else
	$(eval COMMIT_NO := unknown)
	$(eval GIT_COMMIT := unknown)
	$(eval GIT_BRANCH := unknown)
	$(eval GIT_BRANCH_CLEAN := unknown)
endif
