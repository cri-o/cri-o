//go:build linux && cgo
// +build linux,cgo
/*
 * Copyright 2022 Jim Ramsay <jramsay@redhat.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include <errno.h>
#include <unistd.h>
#include <stdio.h>
#include <stdlib.h>
#include <fcntl.h>
#include <sched.h>
#include "kubensmnt.h"

char* getKubeNsMnt() {
	return getenv(KUBENS_ENVNAME);
}

int joinMountNamespace(const char* namespace, char* errorMessage, size_t errlen) {
	errorMessage[0] = '\0';
	if (namespace == NULL) {
		// No namespace requested; silently return
		return 0;
	}

	int fd = open(namespace, O_RDONLY);
	if (fd == -1) {
		snprintf(errorMessage, errlen, "Could not open mount namespace \"%s\": %m", namespace);
		return -1;
	}

	int res = setns(fd, CLONE_NEWNS);
	if (res == -1) {
		snprintf(errorMessage, errlen, "Could not join mount namespace \"%s\": %m", namespace);
		// Fallthrough to clean up fd
	}

	close(fd);
	return res;
}
