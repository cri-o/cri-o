// +build tools

package tools

import (
	_ "github.com/containerd/release-tool"
	_ "github.com/cpuguy83/go-md2man"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "github.com/psampaz/go-mod-outdated"
	_ "github.com/vbatts/git-validation"
	_ "k8s.io/release/cmd/release-notes"
	_ "mvdan.cc/sh/v3/cmd/shfmt"
)
