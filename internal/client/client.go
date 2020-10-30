package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/cri-o/cri-o/pkg/types"
	"github.com/cri-o/cri-o/server"
	json "github.com/json-iterator/go"
)

const (
	maxUnixSocketPathSize = len(syscall.RawSockaddrUnix{}.Path)
)

// CrioClient is an interface to get information from crio daemon endpoint.
type CrioClient interface {
	DaemonInfo() (types.CrioInfo, error)
	ContainerInfo(string) (*types.ContainerInfo, error)
	ConfigInfo() (string, error)
}

type crioClientImpl struct {
	client         *http.Client
	crioSocketPath string
}

func configureUnixTransport(tr *http.Transport, proto, addr string) error {
	if len(addr) > maxUnixSocketPathSize {
		return fmt.Errorf("unix socket path %q is too long", addr)
	}
	// No need for compression in local communications.
	tr.DisableCompression = true
	tr.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.DialTimeout(proto, addr, 32*time.Second)
	}
	return nil
}

// New returns a crio client
func New(crioSocketPath string) (CrioClient, error) {
	tr := new(http.Transport)
	if err := configureUnixTransport(tr, "unix", crioSocketPath); err != nil {
		return nil, err
	}
	c := &http.Client{
		Transport: tr,
	}
	return &crioClientImpl{
		client:         c,
		crioSocketPath: crioSocketPath,
	}, nil
}

func (c *crioClientImpl) getRequest(path string) (*http.Request, error) {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	// For local communications over a unix socket, it doesn't matter what
	// the host is. We just need a valid and meaningful host name.
	req.Host = "crio"
	req.URL.Host = c.crioSocketPath
	req.URL.Scheme = "http"
	return req, nil
}

// DaemonInfo return cri-o daemon info from the cri-o
// info endpoint.
func (c *crioClientImpl) DaemonInfo() (types.CrioInfo, error) {
	info := types.CrioInfo{}
	req, err := c.getRequest(server.InspectInfoEndpoint)
	if err != nil {
		return info, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&info)
	return info, err
}

// ContainerInfo returns container info by querying
// the cri-o container endpoint.
func (c *crioClientImpl) ContainerInfo(id string) (*types.ContainerInfo, error) {
	req, err := c.getRequest(server.InspectContainersEndpoint + "/" + id)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	cInfo := types.ContainerInfo{}
	if err := json.NewDecoder(resp.Body).Decode(&cInfo); err != nil {
		return nil, err
	}
	return &cInfo, nil
}

// ConfigInfo returns current config as TOML string
func (c *crioClientImpl) ConfigInfo() (string, error) {
	req, err := c.getRequest(server.InspectConfigEndpoint)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
