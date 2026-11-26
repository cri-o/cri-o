//go:build linux

package cgmgr

import (
	"reflect"
	"testing"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

func TestCPUQuotaProperties(t *testing.T) {
	quotaProp := func(usecPerSecond uint64) systemdDbus.Property {
		return systemdDbus.Property{
			Name:  "CPUQuotaPerSecUSec",
			Value: dbus.MakeVariant(usecPerSecond),
		}
	}
	periodProp := func(usec uint64) systemdDbus.Property {
		return systemdDbus.Property{Name: "CPUQuotaPeriodUSec", Value: dbus.MakeVariant(usec)}
	}

	for _, tc := range []struct {
		description string
		cpu         *rspec.LinuxCPU
		expected    []systemdDbus.Property
	}{
		{
			description: "nothing set",
			cpu:         &rspec.LinuxCPU{},
		},
		{
			description: "quota of the default period",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(3500))},
			expected:    []systemdDbus.Property{quotaProp(35000)},
		},
		{
			description: "quota of a period of 50ms, still 35 millicores",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(1750)), Period: new(uint64(50000))},
			expected:    []systemdDbus.Property{periodProp(50000), quotaProp(35000)},
		},
		{
			description: "quota of a period of 200ms, still 35 millicores",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(7000)), Period: new(uint64(200000))},
			expected:    []systemdDbus.Property{periodProp(200000), quotaProp(35000)},
		},
		{
			description: "period without a quota",
			cpu:         &rspec.LinuxCPU{Period: new(uint64(50000))},
			expected:    []systemdDbus.Property{periodProp(50000)},
		},
		{
			description: "period of zero is the default period",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(3500)), Period: new(uint64(0))},
			expected:    []systemdDbus.Property{quotaProp(35000)},
		},
		{
			description: "no quota property for a quota of zero",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(0))},
		},
		{
			description: "no quota property for an unlimited quota",
			cpu:         &rspec.LinuxCPU{Quota: new(int64(-1))},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			actual := cpuQuotaProperties(tc.cpu)

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
