package device

import (
	"fmt"
	"strings"

	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// DeviceAnnotationDelim is the character
// used to separate devices in the annotation
// `io.kubernetes.cri-o.Devices`.
const DeviceAnnotationDelim = ","

// Config is the internal device configuration
// it holds onto the contents of the additional_devices
// field, allowing admins to configure devices that are given
// to all containers.
type Config struct {
	devices []Device
}

// Device holds the runtime spec
// fields needed for a device.
type Device struct {
	Device   rspec.LinuxDevice
	Resource rspec.LinuxDeviceCgroup
}

// New creates a new device Config.
func New() *Config {
	return &Config{
		devices: make([]Device, 0),
	}
}

// LoadDevices takes a slice of strings of additional_devices
// specified in the config.
// It saves the resulting Device structs, so they are
// processed once and used later.
func (d *Config) LoadDevices(devsFromConfig []string) error {
	devs, err := devicesFromStrings(devsFromConfig, nil)
	if err != nil {
		return err
	}

	d.devices = devs

	return nil
}

// DevicesFromAnnotation takes an annotation string of the form
// io.kubernetes.cri-o.Device=$PATH:$PATH:$MODE,$PATH...
// and returns a Device object that can be passed to a create config.
func DevicesFromAnnotation(annotation string, allowedDevices []string) ([]Device, error) {
	allowedMap := make(map[string]struct{})
	for _, d := range allowedDevices {
		allowedMap[d] = struct{}{}
	}

	return devicesFromStrings(strings.Split(annotation, DeviceAnnotationDelim), allowedMap)
}

// devicesFromStrings takes a slice of strings in the form $PATH{:$PATH}{:$MODE}
// Where the first path is the path to the device on the host
// The second is where the device will be put in the container (optional)
// and the third is the mode the device will be mounted with (optional)
// It returns a slice of Device structs, ready to be saved or given to a container
// runtime spec generator.
func devicesFromStrings(devsFromConfig []string, allowedDevices map[string]struct{}) ([]Device, error) {
	linuxdevs := make([]Device, 0, len(devsFromConfig))

	for _, d := range devsFromConfig {
		// ignore empty entries
		if d == "" {
			continue
		}

		src, dst, permissions, err := parseDevice(d)
		if err != nil {
			return nil, err
		}

		if allowedDevices != nil {
			if _, ok := allowedDevices[src]; !ok {
				return nil, fmt.Errorf("device %s is specified but is not in allowed_devices", src)
			}
		}
		// ParseDevice does not check the destination is in /dev,
		// but it should be checked
		if !strings.HasPrefix(dst, "/dev/") {
			return nil, fmt.Errorf("invalid device mode: %s", dst)
		}

		dev, err := devices.DeviceFromPath(src, permissions)
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid device: %w", src, err)
		}

		dev.Path = dst

		linuxdevs = append(linuxdevs,
			Device{
				Device: rspec.LinuxDevice{
					Path:     dev.Path,
					Type:     string(dev.Type),
					Major:    dev.Major,
					Minor:    dev.Minor,
					FileMode: &dev.FileMode,
					UID:      &dev.Uid,
					GID:      &dev.Gid,
				},
				Resource: rspec.LinuxDeviceCgroup{
					Allow:  true,
					Type:   string(dev.Type),
					Major:  &dev.Major,
					Minor:  &dev.Minor,
					Access: permissions,
				},
			})
	}

	return linuxdevs, nil
}

// Devices returns the devices saved in the Config.
func (d *Config) Devices() []Device {
	return d.devices
}

// ParseDevice parses device mapping string to a src, dest & permissions string.
func parseDevice(device string) (src, dst, permissions string, err error) {
	permissions = "rwm"

	arr := strings.Split(device, ":")
	switch len(arr) {
	case 3:
		if !isValidDeviceMode(arr[2]) {
			return "", "", "", fmt.Errorf("invalid device mode: %s", arr[2])
		}

		permissions = arr[2]

		fallthrough
	case 2:
		if isValidDeviceMode(arr[1]) {
			permissions = arr[1]
		} else {
			if arr[1] != "" && arr[1][0] != '/' {
				return "", "", "", fmt.Errorf("invalid device mode: %s", arr[1])
			}

			dst = arr[1]
		}

		fallthrough
	case 1:
		src = arr[0]
	default:
		return "", "", "", fmt.Errorf("invalid device specification: %s", device)
	}

	if dst == "" {
		dst = src
	}

	return src, dst, permissions, nil
}

// IsValidDeviceMode checks if the mode for device is valid or not.
// IsValid mode is a composition of r (read), w (write), and m (mknod).
func isValidDeviceMode(mode string) bool {
	legalDeviceMode := map[rune]bool{
		'r': true,
		'w': true,
		'm': true,
	}

	if mode == "" {
		return false
	}

	for _, c := range mode {
		if !legalDeviceMode[c] {
			return false
		}

		legalDeviceMode[c] = false
	}

	return true
}
