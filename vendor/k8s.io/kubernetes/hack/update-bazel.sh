#!/usr/bin/env bash
# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

export KUBE_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${KUBE_ROOT}/hack/lib/init.sh"

git config http.https://gopkg.in.followRedirects true

go get -u gopkg.in/mikedanese/gazel.v14/gazel

for path in ${GOPATH//:/ }; do
    if [[ -e "${path}/bin/gazel" ]]; then
      gazel="${path}/bin/gazel"
      break
    fi
done
if [[ -z "${gazel:-}" ]]; then
  echo "Couldn't find gazel on the GOPATH."
  exit 1
fi
"${gazel}" -root="$(kube::realpath ${KUBE_ROOT})"
