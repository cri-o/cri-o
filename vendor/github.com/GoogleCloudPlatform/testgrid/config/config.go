/*
Copyright 2019 The Kubernetes Authors.

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

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	multierror "github.com/hashicorp/go-multierror"
)

// MissingFieldError is an error that includes the missing field.
type MissingFieldError struct {
	Field string
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("field missing or unset: %s", e.Field)
}

// DuplicateNameError is an error that includes the duplicate name.
type DuplicateNameError struct {
	Name   string
	Entity string
}

func (e DuplicateNameError) Error() string {
	return fmt.Sprintf("found duplicate name after normalizing: (%s) %s", e.Entity, e.Name)
}

// MissingEntityError is an error that includes the missing entity.
type MissingEntityError struct {
	Name   string
	Entity string
}

func (e MissingEntityError) Error() string {
	return fmt.Sprintf("could not find the referenced (%s) %s", e.Entity, e.Name)
}

// ConfigError is an error for invalid configuration that includes what entity errored.
type ConfigError struct {
	Name    string
	Entity  string
	Message string
}

func (e ConfigError) Error() string {
	return fmt.Sprintf("configuration error for (%s) %s: %s", e.Entity, e.Name, e.Message)
}

// normalize lowercases, and removes all non-alphanumeric characters from a string.
func normalize(s string) string {
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	s = regex.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	return s
}

// validateUnique checks that a list has no duplicate normalized entries.
func validateUnique(items []string, entity string) error {
	var mErr error
	set := map[string]bool{}
	for _, item := range items {
		s := normalize(item)
		_, ok := set[s]
		if ok {
			mErr = multierror.Append(mErr, DuplicateNameError{s, entity})
		} else {
			set[s] = true
		}
	}
	return mErr
}

func validateAllUnique(c *configpb.Configuration) error {
	var mErr error
	if c == nil {
		return multierror.Append(mErr, errors.New("got an empty config.Configuration"))
	}
	var tgNames []string
	for _, tg := range c.GetTestGroups() {
		if err := validateName(tg.GetName()); err != nil {
			mErr = multierror.Append(mErr, &ConfigError{tg.GetName(), "TestGroup", err.Error()})
		}
		tgNames = append(tgNames, tg.GetName())
	}
	// Test Group names must be unique.
	if err := validateUnique(tgNames, "TestGroup"); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	var dashNames []string
	for _, dash := range c.GetDashboards() {
		if err := validateName(dash.Name); err != nil {
			mErr = multierror.Append(mErr, &ConfigError{dash.GetName(), "Dashboard", err.Error()})
		}
		dashNames = append(dashNames, dash.Name)
		var tabNames []string
		for _, tab := range dash.GetDashboardTab() {
			if err := validateName(tab.Name); err != nil {
				mErr = multierror.Append(mErr, &ConfigError{tab.Name, "DashboardTab", err.Error()})
			}
			tabNames = append(tabNames, tab.Name)
		}
		// Dashboard Tab names must be unique within a Dashboard.
		if err := validateUnique(tabNames, "DashboardTab"); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}
	// Dashboard names must be unique within Dashboards.
	if err := validateUnique(dashNames, "Dashboard"); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	var dgNames []string
	for _, dg := range c.GetDashboardGroups() {
		if err := validateName(dg.Name); err != nil {
			mErr = multierror.Append(mErr, &ConfigError{dg.Name, "DashboardGroup", err.Error()})
		}
		dgNames = append(dgNames, dg.Name)
	}
	// Dashboard Group names must be unique within Dashboard Groups.
	if err := validateUnique(dgNames, "DashboardGroup"); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// Names must also be unique within DashboardGroups AND Dashbaords.
	if err := validateUnique(append(dashNames, dgNames...), "Dashboard/DashboardGroup"); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	return mErr
}

func validateReferencesExist(c *configpb.Configuration) error {
	var mErr error
	if c == nil {
		return multierror.Append(mErr, errors.New("got an empty config.Configuration"))
	}

	tgNames := map[string]bool{}
	for _, tg := range c.GetTestGroups() {
		tgNames[tg.GetName()] = true
	}
	tgInTabs := map[string]bool{}
	for _, dash := range c.GetDashboards() {
		for _, tab := range dash.DashboardTab {
			tabTg := tab.TestGroupName
			tgInTabs[tabTg] = true
			// Verify that each Test Group referenced by a Dashboard Tab exists.
			if _, ok := tgNames[tabTg]; !ok {
				mErr = multierror.Append(mErr, MissingEntityError{tabTg, "TestGroup"})
			}
		}
	}
	// Likewise, each Test Group must be referenced by a Dashboard Tab, so each Test Group gets displayed.
	for tgName := range tgNames {
		if _, ok := tgInTabs[tgName]; !ok {
			mErr = multierror.Append(mErr, ConfigError{tgName, "TestGroup", "Each Test Group must be referenced by at least 1 Dashboard Tab."})
		}
	}

	dashNames := map[string]bool{}
	for _, dash := range c.GetDashboards() {
		dashNames[dash.Name] = true
	}
	dashToDg := map[string]bool{}
	for _, dg := range c.GetDashboardGroups() {
		for _, name := range dg.DashboardNames {
			dgDash := name
			if _, ok := dashNames[dgDash]; !ok {
				// The Dashboards each Dashboard Group references must exist.
				mErr = multierror.Append(mErr, MissingEntityError{dgDash, "Dashboard"})
			} else if _, ok = dashToDg[dgDash]; ok {
				mErr = multierror.Append(mErr, ConfigError{dgDash, "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."})
			} else {
				dashToDg[dgDash] = true
			}
		}
	}
	return mErr
}

// validateName validates an entity name is non-empty and contains no prefix that overlaps with a
// TestGrid file prefix, post-normalization.
func validateName(s string) error {
	name := normalize(s)
	if name == "" {
		return errors.New("normalized name can't be empty")
	}

	invalidPrefixes := []string{"dashboard", "alerter", "summary", "bugs"}
	for _, prefix := range invalidPrefixes {
		if strings.HasPrefix(name, prefix) {
			return fmt.Errorf("normalized name can't be prefixed with any of %v", invalidPrefixes)
		}
	}

	return nil
}

// validateEmails is a very basic check that each address in a comma-separated list is valid.
func validateEmails(addresses string) error {
	// Each address should have exactly one @ symbol, with characters before and after.
	regex := regexp.MustCompile("^[^@]+@[^@]+$")
	invalid := []string{}
	for _, address := range strings.Split(addresses, ",") {
		match := regex.Match([]byte(address))
		if !match {
			invalid = append(invalid, address)
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf("bad emails %v specified in '%s'; an email address should have exactly one at (@) symbol)", invalid, addresses)
	}
	return nil
}

func validateTestGroup(tg *configpb.TestGroup) error {
	var mErr error
	if tg == nil {
		return multierror.Append(mErr, errors.New("got an empty TestGroup"))
	}
	// Check that required fields are a non-zero-value.
	if tg.GetGcsPrefix() == "" {
		mErr = multierror.Append(mErr, errors.New("gcs_prefix can't be empty"))
	}
	if tg.GetDaysOfResults() <= 0 {
		mErr = multierror.Append(mErr, errors.New("days_of_results should be positive"))
	}
	if tg.GetNumColumnsRecent() <= 0 {
		mErr = multierror.Append(mErr, errors.New("num_columns_recent should be positive"))
	}

	// Regexes should be valid.
	if _, err := regexp.Compile(tg.GetCommitOverrideLabelPattern()); err != nil {
		mErr = multierror.Append(mErr, fmt.Errorf("commit_override_label_pattern doesn't compile: %v", err))
	}
	if _, err := regexp.Compile(tg.GetTestMethodMatchRegex()); err != nil {
		mErr = multierror.Append(mErr, fmt.Errorf("test_method_match_regex doesn't compile: %v", err))
	}

	// Email address for alerts should be valid.
	if tg.GetAlertMailToAddresses() != "" {
		if err := validateEmails(tg.GetAlertMailToAddresses()); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Test metadata options should be reasonable, valid values.
	metadataOpts := tg.GetTestMetadataOptions()
	for _, opt := range metadataOpts {
		if opt.GetBugComponent() <= 0 {
			mErr = multierror.Append(mErr, errors.New("bug_component is required"))
		}
		if opt.GetMessageRegex() == "" && opt.GetTestNameRegex() == "" {
			mErr = multierror.Append(mErr, errors.New("at least one of message_regex or test_name_regex must be specified"))
		}
		if _, err := regexp.Compile(opt.GetMessageRegex()); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("message_regex doesn't compile: %v", err))
		}
		if _, err := regexp.Compile(opt.GetTestNameRegex()); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("test_name_regex doesn't compile: %v", err))
		}
	}

	for _, notification := range tg.GetNotifications() {
		if notification.GetSummary() == "" {
			mErr = multierror.Append(mErr, errors.New("summary is required"))
		}
	}

	annotations := tg.GetTestAnnotations()
	for _, annotation := range annotations {
		if annotation.GetPropertyName() == "" {
			mErr = multierror.Append(mErr, errors.New("property_name is required"))
		}
		if annotation.GetShortText() == "" || len(annotation.GetShortText()) >= 5 {
			mErr = multierror.Append(mErr, errors.New("short_text must be 1-5 characters long"))
		}
	}

	fallbackConfigSettingSet := tg.GetFallbackGrouping() == configpb.TestGroup_FALLBACK_GROUPING_CONFIGURATION_VALUE
	fallbackConfigValueSet := tg.GetFallbackGroupingConfigurationValue() != ""
	if fallbackConfigSettingSet != fallbackConfigValueSet {
		mErr = multierror.Append(
			mErr,
			errors.New("fallback_grouping_configuration_value and fallback_grouping = FALLBACK_GROUPING_CONFIGURATION_VALUE require each other"),
		)
	}

	// For each defined column_header, verify it has exactly one value set.
	for idx, header := range tg.GetColumnHeader() {
		if cv, p, l := header.ConfigurationValue, header.Property, header.Label; cv == "" && p == "" && l == "" {
			mErr = multierror.Append(mErr, &ConfigError{tg.GetName(), "TestGroup", fmt.Sprintf("Column Header %d is empty", idx)})
		} else if cv != "" && (p != "" || l != "") || p != "" && (cv != "" || l != "") {
			mErr = multierror.Append(
				mErr,
				fmt.Errorf("Column Header %d must only set one value, got configuration_value: %q, property: %q, label: %q", idx, cv, p, l),
			)
		}

	}

	// test_name_config should have a matching number of format strings and name elements.
	if tg.GetTestNameConfig() != nil {
		nameFormat := tg.GetTestNameConfig().GetNameFormat()
		nameElements := tg.GetTestNameConfig().GetNameElements()

		if len(nameElements) == 0 {
			mErr = multierror.Append(mErr, errors.New("TestNameConfig.NameElements must be specified"))
		}

		if nameFormat == "" {
			mErr = multierror.Append(mErr, errors.New("TestNameConfig.NameFormat must be specified"))
		} else {
			if got, want := len(nameElements), strings.Count(nameFormat, "%"); got != want {
				mErr = multierror.Append(
					mErr,
					fmt.Errorf("TestNameConfig has %d elements, format %s wants %d", got, nameFormat, want),
				)
			}
			elements := make([]interface{}, 0)
			for range nameElements {
				elements = append(elements, "")
			}
			s := fmt.Sprintf(nameFormat, elements...)
			if strings.Contains(s, "%!") {
				return fmt.Errorf("number of format strings and name_elements must match; got %s (%d)", s, len(elements))
			}
		}
	}

	return mErr
}

func validateDashboardTab(dt *configpb.DashboardTab) error {
	var mErr error
	if dt == nil {
		return multierror.Append(mErr, errors.New("got an empty DashboardTab"))
	}

	// Check that required fields are a non-zero-value.
	if dt.GetTestGroupName() == "" {
		mErr = multierror.Append(mErr, errors.New("test_group_name can't be empty"))
	}

	// A Dashboard Tab can't be named the same as the default 'Summary' tab.
	if dt.GetName() == "Summary" {
		mErr = multierror.Append(mErr, errors.New("tab can't be named 'Summary'"))
	}

	// TabularNamesRegex should be valid and have capture groups defined.
	if dt.GetTabularNamesRegex() != "" {
		regex, err := regexp.Compile(dt.GetTabularNamesRegex())
		if err != nil {
			mErr = multierror.Append(
				mErr,
				fmt.Errorf("invalid regex %s: %v", dt.GetTabularNamesRegex(), err))
		} else {
			if regex.NumSubexp() != len(regex.SubexpNames()) {
				mErr = multierror.Append(mErr, errors.New("all tabular_name_regex capture groups must be named"))
			}
			if len(regex.SubexpNames()) < 1 {
				mErr = multierror.Append(mErr, errors.New("tabular_name_regex requires at least one capture group"))
			}
		}
	}

	// Email address for alerts should be valid.
	if dt.GetAlertOptions().GetAlertMailToAddresses() != "" {
		if err := validateEmails(dt.GetAlertOptions().GetAlertMailToAddresses()); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr
}

func validateEntityConfigs(c *configpb.Configuration) error {
	var mErr error
	if c == nil {
		return multierror.Append(mErr, errors.New("got an empty config.Configuration"))
	}

	// At the moment, don't need to further validate Dashboards or DashboardGroups.
	for _, tg := range c.GetTestGroups() {
		if err := validateTestGroup(tg); err != nil {
			mErr = multierror.Append(mErr, &ConfigError{tg.GetName(), "TestGroup", err.Error()})
		}
	}

	for _, d := range c.GetDashboards() {
		for _, dt := range d.DashboardTab {
			if err := validateDashboardTab(dt); err != nil {
				mErr = multierror.Append(mErr, &ConfigError{dt.GetName(), "DashboardTab", err.Error()})
			}
		}
	}

	return mErr
}

// Validate checks that a configuration is well-formed.
func Validate(c *configpb.Configuration) error {
	var mErr error
	if c == nil {
		return multierror.Append(mErr, errors.New("got an empty config.Configuration"))
	}

	// TestGrid requires at least 1 TestGroup and 1 Dashboard in order to do anything.
	if len(c.GetTestGroups()) == 0 {
		return multierror.Append(mErr, MissingFieldError{"TestGroups"})
	}
	if len(c.GetDashboards()) == 0 {
		return multierror.Append(mErr, MissingFieldError{"Dashboards"})
	}

	// Names have to be unique (after normalizing) within types of entities, to prevent storing
	// duplicate state on updates and confusion between similar names.
	// Entity names can't be empty or start with the same prefix as a TestGrid file type.
	if err := validateAllUnique(c); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// The entity that an entity references must exist.
	if err := validateReferencesExist(c); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// Validate individual entities have reasonable, well-formed options set.
	if err := validateEntityConfigs(c); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	return mErr
}

// Unmarshal reads a protocol buffer into memory
func Unmarshal(r io.Reader) (*configpb.Configuration, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg configpb.Configuration
	if err = proto.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}
	return &cfg, nil
}

// MarshalText writes a text version of the parsed configuration to the supplied io.Writer.
// Returns an error if config is invalid or writing failed.
func MarshalText(c *configpb.Configuration, w io.Writer) error {
	if c == nil {
		return errors.New("got an empty config.Configuration")
	}
	if err := Validate(c); err != nil {
		return err
	}
	return proto.MarshalText(w, c)
}

// MarshalBytes returns the wire-encoded protobuf data for the parsed configuration.
// Returns an error if config is invalid or encoding failed.
func MarshalBytes(c *configpb.Configuration) ([]byte, error) {
	if c == nil {
		return nil, errors.New("got an empty config.Configuration")
	}
	if err := Validate(c); err != nil {
		return nil, err
	}
	return proto.Marshal(c)
}

// ReadGCS reads the config from gcs and unmarshals it into a Configuration struct.
func ReadGCS(ctx context.Context, obj *storage.ObjectHandle) (*configpb.Configuration, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %v", err)
	}
	return Unmarshal(r)
}

// ReadPath reads the config from the specified local file path.
func ReadPath(path string) (*configpb.Configuration, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	return Unmarshal(f)
}

// Read will read the Configuration proto message from a local or gs:// path.
//
// The ctx and client are only relevant when path refers to GCS.
func Read(path string, ctx context.Context, client *storage.Client) (*configpb.Configuration, error) {
	if strings.HasPrefix(path, "gs://") {
		var gcsPath gcs.Path
		if err := gcsPath.Set(path); err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		return ReadGCS(ctx, client.Bucket(gcsPath.Bucket()).Object(gcsPath.Object()))
	}
	return ReadPath(path)
}

// FindTestGroup returns the configpb.TestGroup proto for a given TestGroup name.
func FindTestGroup(name string, cfg *configpb.Configuration) *configpb.TestGroup {
	if cfg == nil {
		return nil
	}
	for _, tg := range cfg.GetTestGroups() {
		if tg.GetName() == name {
			return tg
		}
	}
	return nil
}

// FindDashboard returns the configpb.Dashboard proto for a given Dashboard name.
func FindDashboard(name string, cfg *configpb.Configuration) *configpb.Dashboard {
	if cfg == nil {
		return nil
	}
	for _, d := range cfg.GetDashboards() {
		if d.Name == name {
			return d
		}
	}
	return nil
}
