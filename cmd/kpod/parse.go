package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

var (
	whiteSpaces  = " \t"
	alphaRegexp  = regexp.MustCompile(`[a-zA-Z]`)
	domainRegexp = regexp.MustCompile(`^(:?(:?[a-zA-Z0-9]|(:?[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9]))(:?\.(:?[a-zA-Z0-9]|(:?[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])))*)\.?\s*$`)
)

// validateMACAddress validates a MAC address.
func validateMACAddress(val string) (string, error) {
	_, err := net.ParseMAC(strings.TrimSpace(val))
	if err != nil {
		return "", err
	}
	return val, nil
}

// validateLink validates that the specified string has a valid link format (containerName:alias).
func validateLink(val string) (string, error) {
	if _, _, err := parseLink(val); err != nil {
		return val, err
	}
	return val, nil
}

// validDeviceMode checks if the mode for device is valid or not.
// Valid mode is a composition of r (read), w (write), and m (mknod).
func validDeviceMode(mode string) bool {
	var legalDeviceMode = map[rune]bool{
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

// validateDevice validates a path for devices
// It will make sure 'val' is in the form:
//    [host-dir:]container-path[:mode]
// It also validates the device mode.
func validateDevice(val string) (string, error) {
	return validatePath(val, validDeviceMode)
}

func validatePath(val string, validator func(string) bool) (string, error) {
	var containerPath string
	var mode string

	if strings.Count(val, ":") > 2 {
		return val, fmt.Errorf("bad format for path: %s", val)
	}

	split := strings.SplitN(val, ":", 3)
	if split[0] == "" {
		return val, fmt.Errorf("bad format for path: %s", val)
	}
	switch len(split) {
	case 1:
		containerPath = split[0]
		val = path.Clean(containerPath)
	case 2:
		if isValid := validator(split[1]); isValid {
			containerPath = split[0]
			mode = split[1]
			val = fmt.Sprintf("%s:%s", path.Clean(containerPath), mode)
		} else {
			containerPath = split[1]
			val = fmt.Sprintf("%s:%s", split[0], path.Clean(containerPath))
		}
	case 3:
		containerPath = split[1]
		mode = split[2]
		if isValid := validator(split[2]); !isValid {
			return val, fmt.Errorf("bad mode specified: %s", mode)
		}
		val = fmt.Sprintf("%s:%s:%s", split[0], containerPath, mode)
	}

	if !path.IsAbs(containerPath) {
		return val, fmt.Errorf("%s is not an absolute path", containerPath)
	}
	return val, nil
}

func validateProto(proto string) bool {
	for _, availableProto := range []string{"tcp", "udp"} {
		if availableProto == proto {
			return true
		}
	}
	return false
}

// validateAttach validates that the specified string is a valid attach option.
func validateAttach(val string) (string, error) {
	s := strings.ToLower(val)
	for _, str := range []string{"stdin", "stdout", "stderr"} {
		if s == str {
			return s, nil
		}
	}
	return val, fmt.Errorf("valid streams are STDIN, STDOUT and STDERR")
}

// validateEnv validates an environment variable and returns it.
// If no value is specified, it returns the current value using os.Getenv.
//
// As on ParseEnvFile and related to #16585, environment variable names
// are not validate what so ever, it's up to application inside docker
// to validate them or not.
func validateEnv(val string) (string, error) {
	arr := strings.Split(val, "=")
	if len(arr) > 1 {
		return val, nil
	}
	if !doesEnvExist(val) {
		return val, nil
	}
	return fmt.Sprintf("%s=%s", val, os.Getenv(val)), nil
}

func doesEnvExist(name string) bool {
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if parts[0] == name {
			return true
		}
	}
	return false
}

// validateExtraHost validates that the specified string is a valid extrahost and returns it.
// ExtraHost is in the form of name:ip where the ip has to be a valid ip (ipv4 or ipv6).
func validateExtraHost(val string) (string, error) {
	// allow for IPv6 addresses in extra hosts by only splitting on first ":"
	arr := strings.SplitN(val, ":", 2)
	if len(arr) != 2 || len(arr[0]) == 0 {
		return "", fmt.Errorf("bad format for add-host: %q", val)
	}
	if _, err := validateIPAddress(arr[1]); err != nil {
		return "", fmt.Errorf("invalid IP address in add-host: %q", arr[1])
	}
	return val, nil
}

// validateIPAddress validates an Ip address.
func validateIPAddress(val string) (string, error) {
	var ip = net.ParseIP(strings.TrimSpace(val))
	if ip != nil {
		return ip.String(), nil
	}
	return "", fmt.Errorf("%s is not an ip address", val)
}

// validateDNSSearch validates domain for resolvconf search configuration.
// A zero length domain is represented by a dot (.).
func validateDNSSearch(val string) (string, error) {
	if val = strings.Trim(val, " "); val == "." {
		return val, nil
	}
	return validateDomain(val)
}

func validateDomain(val string) (string, error) {
	if alphaRegexp.FindString(val) == "" {
		return "", fmt.Errorf("%s is not a valid domain", val)
	}
	ns := domainRegexp.FindSubmatch([]byte(val))
	if len(ns) > 0 && len(ns[1]) < 255 {
		return string(ns[1]), nil
	}
	return "", fmt.Errorf("%s is not a valid domain", val)
}

// validateLabel validates that the specified string is a valid label, and returns it.
// Labels are in the form on key=value.
func validateLabel(val string) (string, error) {
	if strings.Count(val, "=") < 1 {
		return "", fmt.Errorf("bad attribute format: %s", val)
	}
	return val, nil
}

// parseDevice parses a device mapping string to a container.DeviceMapping struct
func parseDevice(device string) (*pb.Device, error) {
	src := ""
	dst := ""
	permissions := "rwm"
	arr := strings.Split(device, ":")
	switch len(arr) {
	case 3:
		permissions = arr[2]
		fallthrough
	case 2:
		if validDeviceMode(arr[1]) {
			permissions = arr[1]
		} else {
			dst = arr[1]
		}
		fallthrough
	case 1:
		src = arr[0]
	default:
		return nil, fmt.Errorf("invalid device specification: %s", device)
	}

	if dst == "" {
		dst = src
	}

	deviceMapping := &pb.Device{
		ContainerPath: dst,
		HostPath:      src,
		Permissions:   permissions,
	}
	return deviceMapping, nil
}

// parseEnvFile reads a file with environment variables enumerated by lines
//
// ``Environment variable names used by the utilities in the Shell and
// Utilities volume of IEEE Std 1003.1-2001 consist solely of uppercase
// letters, digits, and the '_' (underscore) from the characters defined in
// Portable Character Set and do not begin with a digit. *But*, other
// characters may be permitted by an implementation; applications shall
// tolerate the presence of such names.''
// -- http://pubs.opengroup.org/onlinepubs/009695399/basedefs/xbd_chap08.html
//
// As of #16585, it's up to application inside docker to validate or not
// environment variables, that's why we just strip leading whitespace and
// nothing more.
func parseEnvFile(filename string) ([]string, error) {
	fh, err := os.Open(filename)
	if err != nil {
		return []string{}, err
	}
	defer fh.Close()

	lines := []string{}
	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		// trim the line from all leading whitespace first
		line := strings.TrimLeft(scanner.Text(), whiteSpaces)
		// line is not empty, and not starting with '#'
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			data := strings.SplitN(line, "=", 2)

			// trim the front of a variable, but nothing else
			variable := strings.TrimLeft(data[0], whiteSpaces)
			if strings.ContainsAny(variable, whiteSpaces) {
				return []string{}, errors.Errorf("variable %q has white spaces, poorly formatted environment", variable)
			}

			if len(data) > 1 {

				// pass the value through, no trimming
				lines = append(lines, fmt.Sprintf("%s=%s", variable, data[1]))
			} else {
				// if only a pass-through variable is given, clean it up.
				lines = append(lines, fmt.Sprintf("%s=%s", strings.TrimSpace(line), os.Getenv(line)))
			}
		}
	}
	return lines, scanner.Err()
}

// parseLink parses and validates the specified string as a link format (name:alias)
func parseLink(val string) (string, string, error) {
	if val == "" {
		return "", "", fmt.Errorf("empty string specified for links")
	}
	arr := strings.Split(val, ":")
	if len(arr) > 2 {
		return "", "", fmt.Errorf("bad format for links: %s", val)
	}
	if len(arr) == 1 {
		return val, val, nil
	}
	// This is kept because we can actually get a HostConfig with links
	// from an already created container and the format is not `foo:bar`
	// but `/foo:/c1/bar`
	if strings.HasPrefix(arr[0], "/") {
		_, alias := path.Split(arr[1])
		return arr[0][1:], alias, nil
	}
	return arr[0], arr[1], nil
}

func parseLoggingOpts(logDriver string, logDriverOpt []string) (map[string]string, error) {
	logOptsMap := convertKVStringsToMap(logDriverOpt)
	if logDriver == "none" && len(logDriverOpt) > 0 {
		return map[string]string{}, errors.Errorf("invalid logging opts for driver %s", logDriver)
	}
	return logOptsMap, nil
}

// takes a local seccomp daemon, reads the file contents for sending to the daemon
func parseSecurityOpts(securityOpts []string) ([]string, error) {
	for key, opt := range securityOpts {
		con := strings.SplitN(opt, "=", 2)
		if len(con) == 1 && con[0] != "no-new-privileges" {
			if strings.Index(opt, ":") != -1 {
				con = strings.SplitN(opt, ":", 2)
			} else {
				return securityOpts, fmt.Errorf("Invalid --security-opt: %q", opt)
			}
		}
		if con[0] == "seccomp" && con[1] != "unconfined" {
			f, err := ioutil.ReadFile(con[1])
			if err != nil {
				return securityOpts, fmt.Errorf("opening seccomp profile (%s) failed: %v", con[1], err)
			}
			b := bytes.NewBuffer(nil)
			if err := json.Compact(b, f); err != nil {
				return securityOpts, fmt.Errorf("compacting json for seccomp profile (%s) failed: %v", con[1], err)
			}
			securityOpts[key] = fmt.Sprintf("seccomp=%s", b.Bytes())
		}
	}

	return securityOpts, nil
}

// parses storage options per container into a map
func parseStorageOpts(storageOpts []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, option := range storageOpts {
		if strings.Contains(option, "=") {
			opt := strings.SplitN(option, "=", 2)
			m[opt[0]] = opt[1]
		} else {
			return nil, errors.Errorf("invalid storage option %q", option)
		}
	}
	return m, nil
}

// parsePortSpecs receives port specs in the format of ip:public:private/proto and parses
// these in to the internal types
func parsePortSpecs(ports []string) ([]*pb.PortMapping, error) {
	var portMappings []*pb.PortMapping
	for _, rawPort := range ports {
		portMapping, err := parsePortSpec(rawPort)
		if err != nil {
			return nil, err
		}

		portMappings = append(portMappings, portMapping...)
	}
	return portMappings, nil
}

// parsePortSpec parses a port specification string into a slice of PortMappings
func parsePortSpec(rawPort string) ([]*pb.PortMapping, error) {
	var proto string
	rawIP, hostPort, containerPort := splitParts(rawPort)
	proto, containerPort = splitProtoPort(containerPort)

	// Strip [] from IPV6 addresses
	ip, _, err := net.SplitHostPort(rawIP + ":")
	if err != nil {
		return nil, fmt.Errorf("Invalid ip address %v: %s", rawIP, err)
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return nil, fmt.Errorf("Invalid ip address: %s", ip)
	}
	if containerPort == "" {
		return nil, fmt.Errorf("No port specified: %s<empty>", rawPort)
	}

	startPort, endPort, err := parsePortRange(containerPort)
	if err != nil {
		return nil, fmt.Errorf("Invalid containerPort: %s", containerPort)
	}

	var startHostPort, endHostPort uint64 = 0, 0
	if len(hostPort) > 0 {
		startHostPort, endHostPort, err = parsePortRange(hostPort)
		if err != nil {
			return nil, fmt.Errorf("Invalid hostPort: %s", hostPort)
		}
	}

	if hostPort != "" && (endPort-startPort) != (endHostPort-startHostPort) {
		// Allow host port range iff containerPort is not a range.
		// In this case, use the host port range as the dynamic
		// host port range to allocate into.
		if endPort != startPort {
			return nil, fmt.Errorf("Invalid ranges specified for container and host Ports: %s and %s", containerPort, hostPort)
		}
	}

	if !validateProto(strings.ToLower(proto)) {
		return nil, fmt.Errorf("invalid proto: %s", proto)
	}

	protocol := pb.Protocol_TCP
	if strings.ToLower(proto) == "udp" {
		protocol = pb.Protocol_UDP
	}

	var ports []*pb.PortMapping
	for i := uint64(0); i <= (endPort - startPort); i++ {
		containerPort = strconv.FormatUint(startPort+i, 10)
		if len(hostPort) > 0 {
			hostPort = strconv.FormatUint(startHostPort+i, 10)
		}
		// Set hostPort to a range only if there is a single container port
		// and a dynamic host port.
		if startPort == endPort && startHostPort != endHostPort {
			hostPort = fmt.Sprintf("%s-%s", hostPort, strconv.FormatUint(endHostPort, 10))
		}

		ctrPort, err := strconv.ParseInt(containerPort, 10, 32)
		if err != nil {
			return nil, err
		}
		hPort, err := strconv.ParseInt(hostPort, 10, 32)
		if err != nil {
			return nil, err
		}

		port := &pb.PortMapping{
			Protocol:      protocol,
			ContainerPort: int32(ctrPort),
			HostPort:      int32(hPort),
			HostIp:        ip,
		}

		ports = append(ports, port)
	}
	return ports, nil
}

// parsePortRange parses and validates the specified string as a port-range (8000-9000)
func parsePortRange(ports string) (uint64, uint64, error) {
	if ports == "" {
		return 0, 0, fmt.Errorf("empty string specified for ports")
	}
	if !strings.Contains(ports, "-") {
		start, err := strconv.ParseUint(ports, 10, 16)
		end := start
		return start, end, err
	}

	parts := strings.Split(ports, "-")
	start, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("Invalid range specified for the Port: %s", ports)
	}
	return start, end, nil
}

// splitParts separates the different parts of rawPort
func splitParts(rawport string) (string, string, string) {
	parts := strings.Split(rawport, ":")
	n := len(parts)
	containerport := parts[n-1]

	switch n {
	case 1:
		return "", "", containerport
	case 2:
		return "", parts[0], containerport
	case 3:
		return parts[0], parts[1], containerport
	default:
		return strings.Join(parts[:n-2], ":"), parts[n-2], containerport
	}
}

// splitProtoPort splits a port in the format of port/proto
func splitProtoPort(rawPort string) (string, string) {
	parts := strings.Split(rawPort, "/")
	l := len(parts)
	if len(rawPort) == 0 || l == 0 || len(parts[0]) == 0 {
		return "", ""
	}
	if l == 1 {
		return "tcp", rawPort
	}
	if len(parts[1]) == 0 {
		return "tcp", parts[0]
	}
	return parts[1], parts[0]
}

// reads a file of line terminated key=value pairs, and overrides any keys
// present in the file with additional pairs specified in the override parameter
func readKVStrings(files []string, override []string) ([]string, error) {
	envVariables := []string{}
	for _, ef := range files {
		parsedVars, err := parseEnvFile(ef)
		if err != nil {
			return nil, err
		}
		envVariables = append(envVariables, parsedVars...)
	}
	// parse the '-e' and '--env' after, to allow override
	envVariables = append(envVariables, override...)

	return envVariables, nil
}

// convertKVStringsToMap converts ["key=value"] to {"key":"value"}
func convertKVStringsToMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		kv := strings.SplitN(value, "=", 2)
		if len(kv) == 1 {
			result[kv[0]] = ""
		} else {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

// NsIpc represents the container ipc stack.
type NsIpc string

// IsPrivate indicates whether the container uses its private ipc stack.
func (n NsIpc) IsPrivate() bool {
	return !(n.IsHost() || n.IsContainer())
}

// IsHost indicates whether the container uses the host's ipc stack.
func (n NsIpc) IsHost() bool {
	return n == "host"
}

// IsContainer indicates whether the container uses a container's ipc stack.
func (n NsIpc) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// Valid indicates whether the ipc stack is valid.
func (n NsIpc) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	case "container":
		if len(parts) != 2 || parts[1] == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// Container returns the name of the container ipc stack is going to be used.
func (n NsIpc) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// NsUser represents userns mode in the container.
type NsUser string

// IsHost indicates whether the container uses the host's userns.
func (n NsUser) IsHost() bool {
	return n == "host"
}

// IsPrivate indicates whether the container uses the a private userns.
func (n NsUser) IsPrivate() bool {
	return !(n.IsHost())
}

// Valid indicates whether the userns is valid.
func (n NsUser) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	default:
		return false
	}
	return true
}

// NsPid represents the pid namespace of the container.
type NsPid string

// IsPrivate indicates whether the container uses its own new pid namespace.
func (n NsPid) IsPrivate() bool {
	return !(n.IsHost() || n.IsContainer())
}

// IsHost indicates whether the container uses the host's pid namespace.
func (n NsPid) IsHost() bool {
	return n == "host"
}

// IsContainer indicates whether the container uses a container's pid namespace.
func (n NsPid) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// Valid indicates whether the pid namespace is valid.
func (n NsPid) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	case "container":
		if len(parts) != 2 || parts[1] == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// Container returns the name of the container whose pid namespace is going to be used.
func (n NsPid) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
