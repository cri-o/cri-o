TEST_TARGETS := test-all test-integration shellcheck

.PHONY: $(TEST_TARGETS)

# Dispatch all test targets to every test Makefile
$(TEST_TARGETS):
	make -C test $@
	make -C utils/kubensenter/test $@
