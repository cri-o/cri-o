package main

import (
	"fmt"
	"testing"
)

func TestValidateMACAddress(t *testing.T) {
	addresses := []string{"01:23:45:67:89:ab",
		"01:23:45:67:89:ab:cd:ef",
		"01:23:45:67:89:ab:cd:ef:00:00:01:23:45:67:89:ab:cd:ef:00:00",
		"01-23-45-67-89-ab",
		"01-23-45-67-89-ab-cd-ef",
		"01-23-45-67-89-ab-cd-ef-00-00-01-23-45-67-89-ab-cd-ef-00-00",
		"0123.4567.89ab",
		"0123.4567.89ab.cdef",
		"0123.4567.89ab.cdef.0000.0123.4567.89ab.cdef.0000"}

	for _, addr := range addresses {
		_, err := validateMACAddress(addr)
		if err != nil {
			t.Fatalf("invalid mac address %q", addr)
		}
	}

	// test invalid mac address
	invalidAddr := "567:78hy:uthg"
	_, err := validateMACAddress(invalidAddr)
	if err == nil {
		t.Fatalf("should have returned an error. %q is not a valid mac address", invalidAddr)
	}
}

func TestValidateLink(t *testing.T) {
	validLink := "containerName:alias"
	_, err := validateLink(validLink)
	if err != nil {
		t.Fatalf("%q is a valid link, but error returned", validLink)
	}

	invalidLinks := []string{"container:alias1:alias2", ""}
	for _, link := range invalidLinks {
		_, err := validateLink(link)
		if err == nil {
			t.Fatalf("should be invalid link %q, but err=nil", link)
		}
	}
}

func TestValidDeviceMode(t *testing.T) {
	validModes := []string{"r", "w", "m"}
	for _, mode := range validModes {
		valid := validDeviceMode(mode)
		if !valid {
			t.Fatalf("should be a valid mode %q", mode)
		}
	}

	invalidModes := []string{"", "a", "b", "blah"}
	for _, mode := range invalidModes {
		valid := validDeviceMode(mode)
		if valid {
			t.Fatalf("should be an invalid mode %q", mode)
		}
	}
}

func TestValidateDevice(t *testing.T) {
	validDevices := []string{"host-dir:/containerPath:w",
		"host:/containerPath:r",
		"host:/containerPath:m",
		"/containerPath",
		"/containerPath:r"}
	for _, dev := range validDevices {
		_, err := validateDevice(dev)
		if err != nil {
			fmt.Println(err)
			t.Fatalf("%q should be a valid device, got invalid", dev)
		}
	}

	invalidDevices := []string{"host:/containerPath:h",
		"containerPath:r",
		"/containerPath:b",
		"containerPath",
		""}
	for _, dev := range invalidDevices {
		_, err := validateDevice(dev)
		if err == nil {
			fmt.Println(err)
			t.Fatalf("%q should be invalid device, got valid", dev)
		}
	}

}

func TestParseLoggingOpts(t *testing.T) {
	for _, validOpts := range []struct {
		logDriver     string
		logDriverOpts []string
	}{
		{
			logDriver:     "testDriver",
			logDriverOpts: []string{"key1=value1, key2=value2"},
		},
		{
			logDriver: "testDriver",
		},
		{
			logDriver: "",
		},
	} {
		_, err := parseLoggingOpts(validOpts.logDriver, validOpts.logDriverOpts)
		if err != nil {
			t.Fatalf("expected valid logging options, got invalid %q", validOpts)
		}
	}

	for _, invalidOpts := range []struct {
		logDriver     string
		logDriverOpts []string
	}{
		{
			logDriver:     "none",
			logDriverOpts: []string{"key1=value1, key2=value2"},
		},
	} {
		_, err := parseLoggingOpts(invalidOpts.logDriver, invalidOpts.logDriverOpts)
		if err == nil {
			t.Fatalf("expected valid logging options, got invalid %q", invalidOpts)
		}
	}
}

func TestParseStorageOpts(t *testing.T) {
	validStorageOpts := []string{"key1=value1", "key2=value2"}
	_, err := parseStorageOpts(validStorageOpts)
	if err != nil {
		t.Fatalf("expected valid storage opts, got invalid instead %q", validStorageOpts)
	}

	invalidStorageOpts := []string{"", "onlyKey", "onlyValue"}
	_, err = parseStorageOpts(invalidStorageOpts)
	if err == nil {
		t.Fatalf("expected invalid storage opts, got valid instead %q", invalidStorageOpts)
	}
}

func TestParsePortSpecs(t *testing.T) {
	validPorts := []string{"123.125.234.123:8888:8888",
		"123.125.234.123:8888:8888/udp",
		"123.125.234.123:8888:8888/tcp",
		"123.125.234.123:8888-8900:8888-8900"}
	_, err := parsePortSpecs(validPorts)
	if err != nil {
		t.Fatalf("expected valid port, got invalid instead %v", err)
	}

	invalidPorts := []string{"12.123.124.123.125:8000:8000",
		"276.567.897.653:8000:8000",
		"567:546:3455:8000:7896",
		"123.124.125.134:8000:9000/blah",
		""}
	_, err = parsePortSpecs(invalidPorts)
	if err == nil {
		t.Fatalf("expected invalid port, got valid instead %v", err)
	}
}
