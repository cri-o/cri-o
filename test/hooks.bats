#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	sed "s|HOOKSCHECK|${HOOKSCHECK}|" "$INTEGRATION_ROOT"/hooks/checkhook.sh > "${HOOKSDIR}"/checkhook.sh
	chmod +x "${HOOKSDIR}"/checkhook.sh
	sed "s|HOOKSDIR|${HOOKSDIR}|" "$INTEGRATION_ROOT"/hooks/checkhook.json > "${HOOKSDIR}"/checkhook.json

}

function teardown() {
	cleanup_test
}

@test "pod test hooks" {
	rm -f "${HOOKSCHECK}"
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
	cat "${HOOKSCHECK}"
}
