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

// EnvName is the name of the environment variable where we check for a mount namespace bindmount.
// If unset, no action is taken.  If set and points at a mount namespace
// bindmount, enter that mount namespace before executing this Go program.  If
// set and an error occurs, the Status function will report an error.
const EnvName = "KUBENSMNT" // Note: Must be manually kept in-sync with the value in kubensmnt.h

// Status returns the configured KubeNS mount namespace filename (or an empty
// string if not configured), and an error representing whether the namespace
// join succeeded.  The actual work of entering a mount namespace must be done
// in a C init-function before any Go threads start up, so all we can do inside
// Go is provide a report of what happened.
func Status() (string, error) {
	// Dispatch to the proper build-time instance (see kubensmnt_*.go)
	return status()
}
