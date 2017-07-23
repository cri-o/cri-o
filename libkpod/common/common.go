package common

import (
	"io"

	cp "github.com/containers/image/copy"
	"github.com/containers/image/types"
)

// GetCopyOptions constructs a new containers/image/copy.Options{} struct from the given parameters
func GetCopyOptions(reportWriter io.Writer, signaturePolicyPath string, srcDockerRegistry, destDockerRegistry *DockerRegistryOptions, signing SigningOptions) *cp.Options {
	if srcDockerRegistry == nil {
		srcDockerRegistry = &DockerRegistryOptions{}
	}
	if destDockerRegistry == nil {
		destDockerRegistry = &DockerRegistryOptions{}
	}
	srcContext := srcDockerRegistry.GetSystemContext(signaturePolicyPath)
	destContext := destDockerRegistry.GetSystemContext(signaturePolicyPath)
	return &cp.Options{
		RemoveSignatures: signing.RemoveSignatures,
		SignBy:           signing.SignBy,
		ReportWriter:     reportWriter,
		SourceCtx:        srcContext,
		DestinationCtx:   destContext,
	}
}

// GetSystemContext Constructs a new containers/image/types.SystemContext{} struct from the given signaturePolicy path
func GetSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{}
	if signaturePolicyPath != "" {
		sc.SignaturePolicyPath = signaturePolicyPath
	}
	return sc
}

// CopyStringStringMap deep copies a map[string]string and returns the result
func CopyStringStringMap(m map[string]string) map[string]string {
	n := map[string]string{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

// IsTrue determines whether the given string equals "true"
func IsTrue(str string) bool {
	return str == "true"
}

// IsFalse determines whether the given string equals "false"
func IsFalse(str string) bool {
	return str == "false"
}

// IsValidBool determines whether the given string equals "true" or "false"
func IsValidBool(str string) bool {
	return IsTrue(str) || IsFalse(str)
}
