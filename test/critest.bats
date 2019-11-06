#!/usr/bin/env bats

load helpers

function setup() {
    setup_test
}

function teardown() {
    cleanup_test
}

@test "run the critest suite" {
    if [[ -z $RUN_CRITEST ]]; then
        skip "critest because RUN_CRITEST is not set"
    fi

    sysctl -w net.ipv4.conf.all.route_localnet=1
    iptables -t nat -I POSTROUTING -s 127.0.0.1 ! -d 127.0.0.1 -j MASQUERADE

    start_crio

    critest --parallel 8 \
                --runtime-endpoint "${CRIO_SOCKET}" \
                --image-endpoint "${CRIO_SOCKET}" \
                --ginkgo.focus="${CRI_FOCUS}" \
                --ginkgo.skip="${CRI_SKIP}" \
                --ginkgo.flakeAttempts=3 >&3

    [ "$status" -eq 0 ]

    stop_crio
}
