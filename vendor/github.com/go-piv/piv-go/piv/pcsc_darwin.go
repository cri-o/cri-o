// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package piv

import "C"

func scCheck(rc C.int) error {
	if rc == rcSuccess {
		return nil
	}
	i := int64(rc)
	if i < 0 {
		// On MacOS, int isn't big enough to handle the return codes so the
		// leading bit becomes a two's complement bit. If the return code is
		// negative, correct this.
		// https://github.com/go-piv/piv-go/issues/53
		i += (1 << 32)
	}
	return &scErr{i}
}

func isRCNoReaders(rc C.int) bool {
	// MacOS does the right thing and doesn't return an error if no smart cards
	// are available.
	return false
}
