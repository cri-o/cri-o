package server

import (
	"io/ioutil"
	"os"
	"testing"
)

const (
	defaultDNSPath = "/etc/resolv.conf"
	testDNSPath    = "fixtures/resolv_test.conf"
	dnsPath        = "fixtures/resolv.conf"
)

func TestParseDNSOptions(t *testing.T) {
	testCases := []struct {
		Servers, Searches, Options []string
		Path                       string
		Want                       string
	}{
		{
			[]string{},
			[]string{},
			[]string{},
			testDNSPath, defaultDNSPath,
		},
		{
			[]string{"cri-o.io", "github.com"},
			[]string{"192.30.253.113", "192.30.252.153"},
			[]string{"timeout:5", "attempts:3"},
			testDNSPath, dnsPath,
		},
	}

	for _, c := range testCases {
		if err := parseDNSOptions(c.Servers, c.Searches,
			c.Options, c.Path); err != nil {
			t.Error(err)
		}

		expect, _ := ioutil.ReadFile(c.Want)
		result, _ := ioutil.ReadFile(c.Path)
		if string(expect) != string(result) {
			t.Errorf("expect %v: \n but got : %v", string(expect), string(result))
		}
		os.Remove(c.Path)
	}
}

func TestSysctlsFromPodAnnotations(t *testing.T) {
	testCases := []struct {
		Annotations   map[string]string
		SafeSysctls   []Sysctl
		UnsafeSysctls []Sysctl
	}{
		{
			map[string]string{
				"foo-":                  "bar",
				SysctlsPodAnnotationKey: "kernel.shmmax=100000000,safe=20000000",
			},
			[]Sysctl{
				{"kernel.shmmax", "100000000"},
				{"safe", "20000000"},
			},
			[]Sysctl{},
		},
		{
			map[string]string{
				UnsafeSysctlsPodAnnotationKey: "kernel.shmmax=10,unsafe=20",
			},
			[]Sysctl{},
			[]Sysctl{
				{"kernel.shmmax", "10"},
				{"unsafe", "20"},
			},
		},
		{
			map[string]string{
				"bar..":                       "42",
				SysctlsPodAnnotationKey:       "kernel.shmmax=20000000,safe=40000000",
				UnsafeSysctlsPodAnnotationKey: "kernel.shmmax=10,unsafe=20",
			},
			[]Sysctl{
				{"kernel.shmmax", "20000000"},
				{"safe", "40000000"},
			},
			[]Sysctl{
				{"kernel.shmmax", "10"},
				{"unsafe", "20"},
			},
		},
	}

	for _, c := range testCases {
		safe, unsafe, err := SysctlsFromPodAnnotations(c.Annotations)
		if err != nil {
			t.Error(err)
		}
		for index, sysctl := range safe {
			if sysctl.Name != safe[index].Name || sysctl.Value != safe[index].Value {
				t.Errorf("Expect safe: %v, but got: %v\n", safe[index], sysctl)
			}
		}
		for index, sysctl := range unsafe {
			if sysctl.Name != unsafe[index].Name || sysctl.Value != unsafe[index].Value {
				t.Errorf("Expect unsafe: %v, but got: %v\n", safe[index], sysctl)
			}
		}
	}
}
