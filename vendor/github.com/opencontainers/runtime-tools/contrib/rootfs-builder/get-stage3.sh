#!/bin/sh
#
# Download the current Gentoo stage3
#
# Copyright (C) 2014-2015 W. Trevor King <wking@tremily.us>
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

MIRROR="${MIRROR:-http://distfiles.gentoo.org/}"
BASE_ARCH_URL="${BASE_ARCH_URL:-${MIRROR}releases/amd64/autobuilds/}"
LATEST=$(wget -O - "${BASE_ARCH_URL}latest-stage3.txt")
DATE=$(echo "${LATEST}" | sed -n 's|/stage3-amd64-[0-9]*[.]tar[.]bz2.*||p')
ARCH_URL="${ARCH_URL:-${BASE_ARCH_URL}${DATE}/}"
STAGE3="${STAGE3:-stage3-amd64-${DATE}.tar.bz2}"
STAGE3_CONTENTS="${STAGE3_CONTENTS:-${STAGE3}.CONTENTS}"
STAGE3_DIGESTS="${STAGE3_DIGESTS:-${STAGE3}.DIGESTS.asc}"

die()
{
	echo "$1"
	exit 1
}

for FILE in "${STAGE3}" "${STAGE3_CONTENTS}" "${STAGE3_DIGESTS}"; do
	if [ ! -f "downloads/${FILE}" ]; then
		wget -O "downloads/${FILE}" "${ARCH_URL}${FILE}"
		if [ "$?" -ne 0 ]; then
			rm -f "downloads/${FILE}" &&
			die "failed to download ${ARCH_URL}${FILE}"
		fi
	fi

	CURRENT=$(echo "${FILE}" | sed "s/${DATE}/current/")
	(
		cd downloads &&
		rm -f "${CURRENT}" &&
		ln -s "${FILE}" "${CURRENT}" ||
		die "failed to link ${CURRENT} -> ${FILE}"
	)
done

