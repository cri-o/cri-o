package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

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
	GoRoutinesInfo(context.Context) (string, error)
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

func (c *crioClientImpl) doGetRequest(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, http.NoBody)
	if err != nil {
		return nil, err
	}
	// For local communications over a unix socket, it doesn't matter what
	// the host is. We just need a valid and meaningful host name.
	req.Host = "crio"
	req.URL.Host = c.crioSocketPath
	req.URL.Scheme = "http"

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do get request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

// DaemonInfo return cri-o daemon info from the cri-o
// info endpoint.
func (c *crioClientImpl) DaemonInfo(ctx context.Context) (types.CrioInfo, error) {
	info := types.CrioInfo{}
	body, err := c.doGetRequest(ctx, server.InspectInfoEndpoint)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(body, &info)
	return info, err
}

// ContainerInfo returns container info by querying
// the cri-o container endpoint.
func (c *crioClientImpl) ContainerInfo(ctx context.Context, id string) (*types.ContainerInfo, error) {
	body, err := c.doGetRequest(ctx, server.InspectContainersEndpoint+"/"+id)
	if err != nil {
		return nil, err
	}
	cInfo := types.ContainerInfo{}
	if err := json.Unmarshal(body, &cInfo); err != nil {
		return nil, err
	}
	return &cInfo, nil
}

// ConfigInfo returns current config as TOML string.
func (c *crioClientImpl) ConfigInfo(ctx context.Context) (string, error) {
	body, err := c.doGetRequest(ctx, server.InspectConfigEndpoint)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// GoRoutinesInfo returns go routine stack as string.
func (c *crioClientImpl) GoRoutinesInfo(ctx context.Context) (string, error) {
	body, err := c.doGetRequest(ctx, server.InspectGoRoutinesEndpoint)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
