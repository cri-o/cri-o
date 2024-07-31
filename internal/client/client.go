package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	json "github.com/json-iterator/go"

	"github.com/cri-o/cri-o/pkg/types"
	"github.com/cri-o/cri-o/server"
)

const (
	maxUnixSocketPathSize = len(syscall.RawSockaddrUnix{}.Path)
)

// CrioClient is an interface to get information from crio daemon endpoint.
type CrioClient interface {
	DaemonInfo(context.Context) (types.CrioInfo, error)
	ContainerInfo(context.Context, string) (*types.ContainerInfo, error)
	ConfigInfo(context.Context) (string, error)
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

// New returns a crio client.
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

func (c *crioClientImpl) getRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, http.NoBody)
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
func (c *crioClientImpl) DaemonInfo(ctx context.Context) (types.CrioInfo, error) {
	info := types.CrioInfo{}
	req, err := c.getRequest(ctx, server.InspectInfoEndpoint)
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
func (c *crioClientImpl) ContainerInfo(ctx context.Context, id string) (*types.ContainerInfo, error) {
	req, err := c.getRequest(ctx, server.InspectContainersEndpoint+"/"+id)
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

// ConfigInfo returns current config as TOML string.
func (c *crioClientImpl) ConfigInfo(ctx context.Context) (string, error) {
	req, err := c.getRequest(ctx, server.InspectConfigEndpoint)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
