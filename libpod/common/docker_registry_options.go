package common

import "github.com/containers/image/types"

// DockerRegistryOptions encapsulates settings that affect how we connect or
// authenticate to a remote registry.
type DockerRegistryOptions struct {
	// DockerRegistryCreds is the user name and password to supply in case
	// we need to pull an image from a registry, and it requires us to
	// authenticate.
	DockerRegistryCreds *types.DockerAuthConfig
	// DockerCertPath is the location of a directory containing CA
	// certificates which will be used to verify the registry's certificate
	// (all files with names ending in ".crt"), and possibly client
	// certificates and private keys (pairs of files with the same name,
	// except for ".cert" and ".key" suffixes).
	DockerCertPath string
	// DockerInsecureSkipTLSVerify turns off verification of TLS
	// certificates and allows connecting to registries without encryption.
	DockerInsecureSkipTLSVerify bool
}

// GetSystemContext constructs a new system context from the given signaturePolicy path and the
// values in the DockerRegistryOptions
func (o DockerRegistryOptions) GetSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{
		SignaturePolicyPath:         signaturePolicyPath,
		DockerAuthConfig:            o.DockerRegistryCreds,
		DockerCertPath:              o.DockerCertPath,
		DockerInsecureSkipTLSVerify: o.DockerInsecureSkipTLSVerify,
	}
	return sc
}
