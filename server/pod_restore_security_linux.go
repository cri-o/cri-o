//go:build linux

/*
Copyright The CRI-O Authors.

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

package server

import (
	"cmp"
	"errors"
	"reflect"
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// Profile references can remain unchanged while the policy resolved by CRI-O
// changes. Compare the saved filter before CRIU can restore the earlier policy.
func validateCheckpointSecurity(captured, destination *rspec.Spec) error {
	if captured == nil || captured.Linux == nil || captured.Process == nil {
		return errors.New("checkpoint archive has no resolved Linux process configuration")
	}

	if destination == nil || destination.Linux == nil || destination.Process == nil {
		return errors.New("restore container has no resolved Linux process configuration")
	}

	if !reflect.DeepEqual(captured.Linux.Seccomp, destination.Linux.Seccomp) {
		return errors.New("restore seccomp policy does not match the checkpoint's resolved policy")
	}

	if !checkpointDevicesEqual(captured.Linux, destination.Linux) {
		return errors.New("restore devices do not match the checkpoint's resolved devices")
	}

	capturedProcess, destinationProcess := captured.Process, destination.Process
	if !checkpointResourceLimitsEqual(capturedProcess.Rlimits, destinationProcess.Rlimits) {
		return errors.New(
			"restore process resource limits do not match the checkpoint's resolved resource limits",
		)
	}

	capturedUser, destinationUser := capturedProcess.User, destinationProcess.User

	capturedUser.AdditionalGids, destinationUser.AdditionalGids = nil, nil
	if !reflect.DeepEqual(capturedUser, destinationUser) ||
		!checkpointSecuritySetsEqual(
			capturedProcess.User.AdditionalGids,
			destinationProcess.User.AdditionalGids,
		) {
		return errors.New(
			"restore process credentials do not match the checkpoint's resolved credentials",
		)
	}

	if capturedProcess.NoNewPrivileges != destinationProcess.NoNewPrivileges ||
		capturedProcess.ApparmorProfile != destinationProcess.ApparmorProfile ||
		capturedProcess.SelinuxLabel != destinationProcess.SelinuxLabel {
		return errors.New(
			"restore process security does not match the checkpoint's resolved security",
		)
	}

	capturedCaps, destinationCaps := capturedProcess.Capabilities, destinationProcess.Capabilities
	if capturedCaps == nil {
		capturedCaps = new(rspec.LinuxCapabilities)
	}

	if destinationCaps == nil {
		destinationCaps = new(rspec.LinuxCapabilities)
	}

	if !checkpointSecuritySetsEqual(capturedCaps.Bounding, destinationCaps.Bounding) ||
		!checkpointSecuritySetsEqual(capturedCaps.Effective, destinationCaps.Effective) ||
		!checkpointSecuritySetsEqual(capturedCaps.Permitted, destinationCaps.Permitted) ||
		!checkpointSecuritySetsEqual(capturedCaps.Inheritable, destinationCaps.Inheritable) ||
		!checkpointSecuritySetsEqual(capturedCaps.Ambient, destinationCaps.Ambient) {
		return errors.New(
			"restore process capabilities do not match the checkpoint's resolved capabilities",
		)
	}

	return nil
}

func checkpointSecuritySetsEqual[T ~uint32 | ~string](captured, destination []T) bool {
	captured, destination = slices.Clone(captured), slices.Clone(destination)
	slices.Sort(captured)
	slices.Sort(destination)

	return slices.Equal(captured, destination)
}

func checkpointResourceLimitsEqual(captured, destination []rspec.POSIXRlimit) bool {
	captured, destination = slices.Clone(captured), slices.Clone(destination)
	compare := func(a, b rspec.POSIXRlimit) int {
		return cmp.Or(
			cmp.Compare(a.Type, b.Type),
			cmp.Compare(a.Hard, b.Hard),
			cmp.Compare(a.Soft, b.Soft),
		)
	}
	slices.SortFunc(captured, compare)
	slices.SortFunc(destination, compare)

	for _, limits := range [][]rspec.POSIXRlimit{captured, destination} {
		for i := 1; i < len(limits); i++ {
			if limits[i-1].Type == limits[i].Type {
				return false
			}
		}
	}

	return slices.Equal(captured, destination)
}

func checkpointDevicesEqual(captured, destination *rspec.Linux) bool {
	capturedDevices, destinationDevices := captured.Devices, destination.Devices
	if len(capturedDevices) == 0 {
		capturedDevices = nil
	}

	if len(destinationDevices) == 0 {
		destinationDevices = nil
	}

	if !reflect.DeepEqual(capturedDevices, destinationDevices) {
		return false
	}

	var capturedRules, destinationRules []rspec.LinuxDeviceCgroup
	if captured.Resources != nil {
		capturedRules = captured.Resources.Devices
	}

	if destination.Resources != nil {
		destinationRules = destination.Resources.Devices
	}

	if len(capturedRules) == 0 {
		capturedRules = nil
	}

	if len(destinationRules) == 0 {
		destinationRules = nil
	}

	return reflect.DeepEqual(capturedRules, destinationRules)
}
