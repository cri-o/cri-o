package utils

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/helpers"
)

const (
	GithubTokenEnvKey    = "GITHUB_TOKEN"
	GitRemoteEnvKey      = "REMOTE"
	OrgEnvKey            = "ORG"
	VersionFile          = "internal/version/version.go"
	DependenciesYAMLFile = "dependencies.yaml"
	SpecFile             = "contrib/test/ci/cri-o.spec"
	BranchPrefix         = "release-"
	VersionPrefix        = "v"
	CrioOrgRepo          = "cri-o"
)

func GetCurrentVersionFromReleaseBranch(
	repo *git.Repo,
	baseBranchName string,
) (res semver.Version, err error) {
	logrus.Infof("Switching to branch: %s", baseBranchName)

	if err := repo.Checkout(baseBranchName); err != nil {
		return res, fmt.Errorf("unable to checkout branch %s: %w", baseBranchName, err)
	}

	versionFromVersionFile, err := GetCurrentVersionFromVersionFile(VersionFile) // returns "x.xx.x"
	if err != nil {
		return res, fmt.Errorf("unable to read latest version: %w", err)
	}

	logrus.Infof("Using version: %s", versionFromVersionFile)

	return ConvertStringToSemver(versionFromVersionFile)
}

func ConvertStringToSemver(tag string) (res semver.Version, err error) {
	sv, err := helpers.TagStringToSemver(strings.TrimSpace(tag))
	if err != nil {
		return res, fmt.Errorf("unable to convert tag %q to semver: %w", tag, err)
	}

	// Clear any pre-release and development suffixes.
	sv.Pre = nil

	return sv, nil
}

// GetCurrentVersionFromVersionFile returns the Version declared in the
// provided version file. Both "const" and "var" declarations are supported, the
// latter allows downstream builds to override the value via -ldflags -X.
func GetCurrentVersionFromVersionFile(versionFile string) (string, error) {
	expr, err := versionFileValue(versionFile, "Version")
	if err != nil {
		return "", err
	}

	return stringLiteral(expr)
}

// GetReleaseMinorVersionsFromVersionFile returns the ReleaseMinorVersions
// declared in the provided version file.
func GetReleaseMinorVersionsFromVersionFile(versionFile string) ([]string, error) {
	expr, err := versionFileValue(versionFile, "ReleaseMinorVersions")
	if err != nil {
		return nil, err
	}

	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, errors.New("ReleaseMinorVersions is not a composite literal")
	}

	res := make([]string, 0, len(lit.Elts))

	for _, elt := range lit.Elts {
		value, err := stringLiteral(elt)
		if err != nil {
			return nil, fmt.Errorf("ReleaseMinorVersions: %w", err)
		}

		res = append(res, value)
	}

	return res, nil
}

// versionFileValue returns the value of the top level const or var declaration
// with the provided name in the version file.
func versionFileValue(versionFile, name string) (ast.Expr, error) {
	file, err := parser.ParseFile(token.NewFileSet(), versionFile, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse version file %s: %w", versionFile, err)
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, ident := range valueSpec.Names {
				if ident.Name == name && i < len(valueSpec.Values) {
					return valueSpec.Values[i], nil
				}
			}
		}
	}

	return nil, fmt.Errorf("unable to find %s in version file: %s", name, versionFile)
}

func stringLiteral(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", errors.New("value is not a string literal")
	}

	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", fmt.Errorf("unquote %s: %w", lit.Value, err)
	}

	return value, nil
}
