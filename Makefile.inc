# vim: set syntax=make:

.PHONY: git-vars
git-vars:
ifeq ($(shell [[ `command -v git` && -d .git ]] && echo true),true)
	$(eval COMMIT_NO :=$(shell git rev-parse HEAD 2> /dev/null || true))
	$(eval GIT_COMMIT := $(if $(shell git status --porcelain --untracked-files=no),"${COMMIT_NO}-dirty","${COMMIT_NO}"))
	$(eval GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
	$(eval GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g"))
	$(eval GIT_MERGE_BASE := $(shell git merge-base origin/master $(shell git rev-parse --abbrev-ref HEAD)))
else
	$(eval COMMIT_NO := unknown)
	$(eval GIT_COMMIT := unknown)
	$(eval GIT_BRANCH := unknown)
	$(eval GIT_BRANCH_CLEAN := unknown)
	$(eval GIT_MERGE_BASE := HEAD^)
endif
