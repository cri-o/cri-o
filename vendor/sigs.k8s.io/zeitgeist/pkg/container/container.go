/*
Copyright 2021 The Kubernetes Authors.

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

package container

import (
	"github.com/google/go-containerregistry/pkg/authn"
	containerregistry "github.com/google/go-containerregistry/pkg/crane"

	"sigs.k8s.io/release-utils/env"
)

const (
	RegistryPassword = "REGISTRY_USER_PASSWORD"
	RegistryUserName = "REGISTRY_USERNAME"
)

// Container is a wrapper around Container related functionality
type Container struct {
	client Client
	Auth   authn.Basic
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . Client
type Client interface {
	ListTags(
		src string,
	) ([]string, error)
}

// New creates a new default container client. Tokens set via the $REGISTRY_USER_PASSWORD
// and $REGISTRY_USERNAME environment variable will result in an authenticated client.
func New() *Container {
	passwd := env.Default(RegistryPassword, "")
	username := env.Default(RegistryUserName, "")

	return &Container{
		Auth: authn.Basic{
			Password: passwd,
			Username: username,
		},
	}
}

// SetClient can be used to manually set the internal Container client
func (c *Container) SetClient(client Client) {
	c.client = client
}

// Client can be used to retrieve the Client type
func (c *Container) Client() Client {
	return c.client
}

// ListTags list all tag for a specific repository
func (c *Container) ListTags(
	src string,
) ([]string, error) {
	if c.Auth.Username != "" && c.Auth.Password != "" {
		opts := containerregistry.WithAuth(&c.Auth)
		return containerregistry.ListTags(src, opts)
	}

	// If Username/Password for the registry aren't supplied
	// it will use the credentials configured in the docker config file.
	return containerregistry.ListTags(src)
}
