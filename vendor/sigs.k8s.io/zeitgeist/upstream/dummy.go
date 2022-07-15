/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package upstream

// Dummy upstream needs no parameters and always returns a latest version of 1.0.0. Can be used for testing.
type Dummy struct {
	Base
}

// LatestVersion always returns 1.0.0
func (upstream Dummy) LatestVersion() (string, error) {
	return "1.0.0", nil
}
