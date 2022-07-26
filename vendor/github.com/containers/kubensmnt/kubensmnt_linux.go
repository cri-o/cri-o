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

package kubensmnt

/*
#cgo CFLAGS: -Wall -D_GNU_SOURCE

#include "kubensmnt.h"

#define ERR_LIMIT 128

char* nsenterConfig = "";
int nsenterResult = -1;
char _errorMessage[ERR_LIMIT];
char* errorMessage = _errorMessage; // Use an intermediary because C.GoString can't work on an array

void __attribute__((constructor)) kubensmnt_init() {
	errorMessage[0] = '\0';
	nsenterConfig = getKubeNsMnt();
	nsenterResult = joinMountNamespace(nsenterConfig, errorMessage, ERR_LIMIT);
}
*/
import "C"

import "errors"

func status() (string, error) {
	config := C.GoString(C.nsenterConfig)
	if C.nsenterResult == -1 {
		return config, errors.New(C.GoString(C.errorMessage))
	}
	return config, nil
}
