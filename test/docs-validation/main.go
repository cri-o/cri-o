package main

import (
	"bytes"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/pkg/config"
)

type entry struct {
	name  string
	tag   string
	value string
}

const (
	crioCLIGoPath  = "internal/criocli/criocli.go"
	crioCLIMdPath  = "docs/crio.8.md"
	crioConfMdPath = "docs/crio.conf.5.md"
)

var (
	// Tags which should be not checked at all.
	excludedTags = []string{
		"plugin_dir",                  // deprecated
		"runtimes",                    // printed as separate table
		"workloads",                   // printed as separate table
		"manage_network_ns_lifecycle", // deprecated
	}

	// Tags where it should not validate the values.
	excludedTagsValue = []string{
		"apparmor_profile", // contains dynamic version number
		"root",             // user dependent
		"runroot",          // user dependent
		"storage_driver",   // user dependent
	}

	// Tags where it should not validate the values.
	excludedCLI = []string{
		"workloads", // too complex an option for a CLI flag
	}

	// Mapping for inconsistencies between tags and CLI arguments.
	tagToCLIOption = map[string]string{
		"network_dir":         "cni-config-dir",
		"plugin_dir":          "cni-plugin-dir",
		"plugin_dirs":         "cni-plugin-dir",
		"insecure_registries": "insecure-registry",
		"log_to_journald":     "log-journald",
		"storage_option":      "storage-opt",
	}
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.SetLevel(logrus.DebugLevel)

	// Setup the test configuration
	cfg, err := config.DefaultConfig()
	if err != nil {
		logrus.Fatalf("Unable to retrieve default config: %v", err)
	}

	// Do the validation
	tagFailed := validateTags(cfg)
	cliFailed := validateCli(cfg)

	// Evaluate
	if tagFailed || cliFailed {
		os.Exit(1)
	}
	logrus.Info("Everything looks fine")
}

func validateTags(cfg *config.Config) (failed bool) {
	// Parse the tags from it
	entries := allEntries(cfg)

	// Open the documentation
	crioConfDoc := openFile(crioConfMdPath)

	// Parse the template into a buffer
	var templateBytes bytes.Buffer
	if err := cfg.WriteTemplate(true, &templateBytes); err != nil {
		logrus.Fatalf("Unable to write template: %v", err)
	}

	// Check if the found toml tags are available within the template and docs
	logrus.Infof(
		"Verifying TOML tags of `config.go` to `TemplateString` and `%s`",
		crioConfMdPath,
	)
	for _, entry := range entries {
		// Skip whitelisted items
		if stringInSlice(entry.tag, excludedTags) {
			logrus.Debugf("Skipping excluded tag `%s`", entry.tag)
			continue
		}

		// Validate the template
		templateMatch, err := regexp.Match(
			entry.tag+` = `+entry.value,
			templateBytes.Bytes(),
		)
		if err != nil || !templateMatch {
			logrus.Errorf(
				"Tag `%s` with expected value `%s` not found in TemplateString",
				entry.tag, entry.value,
			)
			failed = true
		}

		// Validate the docs
		docsMatch, err := regexp.Match(
			`\*\*`+entry.tag+`\*\*=`+entry.value,
			crioConfDoc,
		)
		if err != nil || !docsMatch {
			logrus.Errorf(
				"Tag `%s` with expected value `%s` not found in `%s`",
				entry.tag, entry.value, crioConfMdPath,
			)
			failed = true
		}
	}

	if failed {
		logrus.Warnf("Tag validation failed")
	} else {
		logrus.Info("Tag validation successful")
	}
	return failed
}

func validateCli(cfg *config.Config) (failed bool) {
	logrus.Infof(
		"Verifying command line arguments of `%s` to `%s`",
		crioCLIGoPath, crioCLIMdPath,
	)

	entries := allEntries(cfg)
	cliGo := openFile(crioCLIGoPath)
	crioCLIDoc := openFile(crioCLIMdPath)

	for _, entry := range entries {
		// Assume a simple tag to CLI option conversion
		cliOption := strings.ReplaceAll(entry.tag, "_", "-")

		// Check if we have to map the tag differently
		if val, ok := tagToCLIOption[entry.tag]; ok {
			logrus.Debugf("Mapping `%s` to `%s`", entry.tag, val)
			cliOption = val
		}

		if stringInSlice(entry.tag, excludedCLI) {
			logrus.Debugf("Skipping excluded CLI entry `%s`", entry.tag)
			continue
		}

		// Lookup the tag
		nameMatches := regexp.
			MustCompile(`.*Name:\s+"(` + cliOption + `.*)",`).
			FindStringSubmatch(string(cliGo))

		// Check if we have enough sub-matches
		if len(nameMatches) != 2 {
			logrus.Errorf(
				"No matching CLI option `%s` found (tag `%s`) in `%s`",
				cliOption, entry.tag, crioCLIGoPath,
			)
			failed = true
			continue
		}

		// Prepare the option to match the expected output
		option := "--" + nameMatches[1]

		// Validate synopsis
		synopsisMatch, err := regexp.Match(`\[`+option+`.*\]`, crioCLIDoc)
		if err != nil || !synopsisMatch {
			logrus.Errorf(
				"CLI option `%s` not found in synopsis of `%s`",
				option, crioCLIMdPath,
			)
			failed = true
		}

		// Validate descriptions
		descriptionMatch, err := regexp.Match(`\*\*`+option+`.*\*\*`, crioCLIDoc)
		if err != nil || !descriptionMatch {
			logrus.Errorf(
				"CLI option `%s` not found in description of `%s`",
				option, crioCLIMdPath,
			)
			failed = true
		}
	}

	if failed {
		logrus.Warnf("CLI validation failed")
	} else {
		logrus.Info("CLI validation successful")
	}
	return failed
}

func openFile(path string) []byte {
	file, err := os.ReadFile(path)
	if err != nil {
		logrus.Fatalf("Unable to open file: %v", err)
	}
	return file
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func allEntries(c *config.Config) []entry {
	entries := &[]entry{}
	recursiveEntries(reflect.ValueOf(*c), entries, map[any]bool{})
	return *entries
}

type stringer interface {
	String() string
}

func recursiveEntries(
	v reflect.Value,
	entries *[]entry,
	seen map[any]bool,
) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.Kind() == reflect.Ptr {
			// Skip private or recursive data
			if !v.CanInterface() || seen[v.Interface()] {
				return
			}
			seen[v.Interface()] = true
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			recursiveEntries(v.Index(i), entries, seen)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			field := t.Field(i)
			tag := strings.TrimSuffix(field.Tag.Get("toml"), ",omitempty")
			name := field.Name

			vv := v.FieldByName(name)
			value := ""
			if !stringInSlice(tag, excludedTagsValue) {
				switch {
				case field.Type.Implements(reflect.TypeOf((*stringer)(nil)).Elem()):
					// We need a checked type assertion to make golangci-lint happy...
					if str, ok := vv.MethodByName("String").Interface().(func() string); ok {
						value = strconv.Quote(str())
						break
					}
					fallthrough
				case vv.Kind() == reflect.Bool:
					value = strconv.FormatBool(vv.Bool())
				case vv.Kind() == reflect.Int64:
					value = strconv.FormatInt(vv.Int(), 10)
				case vv.Kind() == reflect.String:
					value = strconv.Quote(vv.String())
				}
			}

			if tag != "" {
				*entries = append(*entries, entry{name, tag, value})
			}

			recursiveEntries(v.Field(i), entries, seen)
		}
	}
}
