[![crio-metrics-exporter docker pipeline](https://github.com/djkormo/cri-o/actions/workflows/docker-build.yaml/badge.svg)](https://github.com/djkormo/cri-o/actions/workflows/docker-build.yaml)



docker build . -t djkormo/crio-metrics-exporter -f Containerfile


go build -o bin/metrics-exporter