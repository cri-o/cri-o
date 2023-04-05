//go:build tools
// +build tools

package tools

import (
	_ "github.com/cpuguy83/go-md2man"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "github.com/psampaz/go-mod-outdated"
	_ "k8s.io/release/cmd/release-notes"
	_ "mvdan.cc/sh/v3/cmd/shfmt"
)
