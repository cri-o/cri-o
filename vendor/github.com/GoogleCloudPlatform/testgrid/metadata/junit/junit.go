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

// Package junit describes the test-infra definition of "junit", and provides
// utilities to parse it.
package junit

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// Suites holds a <testsuites/> list of Suite results
type Suites struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []Suite  `xml:"testsuite"`
}

// Suite holds <testsuite/> results
type Suite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Suites   []Suite  `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Time     float64  `xml:"time,attr"` // Seconds
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Results  []Result `xml:"testcase"`
	/*
	* <properties><property name="go.version" value="go1.8.3"/></properties>
	 */
}

// Property defines the xml element that stores additional metrics about each benchmark.
type Property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Properties defines the xml element that stores the list of properties that are associated with one benchmark.
type Properties struct {
	PropertyList []Property `xml:"property"`
}

// Result holds <testcase/> results
type Result struct {
	Name       string      `xml:"name,attr"`
	Time       float64     `xml:"time,attr"`
	ClassName  string      `xml:"classname,attr"`
	Failure    *string     `xml:"failure,omitempty"`
	Output     *string     `xml:"system-out,omitempty"`
	Error      *string     `xml:"system-err,omitempty"`
	Skipped    *string     `xml:"skipped,omitempty"`
	Properties *Properties `xml:"properties,omitempty"`
}

// SetProperty adds the specified property to the Result or replaces the
// existing value if a property with that name already exists.
func (r *Result) SetProperty(name, value string) {
	if r.Properties == nil {
		r.Properties = &Properties{}
	}
	for i, existing := range r.Properties.PropertyList {
		if existing.Name == name {
			r.Properties.PropertyList[i].Value = value
			return
		}
	}
	// Didn't find an existing property. Add a new one.
	r.Properties.PropertyList = append(
		r.Properties.PropertyList,
		Property{
			Name:  name,
			Value: value,
		},
	)
}

// Message extracts the message for the junit test case.
//
// Will use the first non-empty <failure/>, <skipped/>, <system-err/>, <system-out/> value.
func (jr Result) Message(max int) string {
	var msg string
	switch {
	case jr.Failure != nil && *jr.Failure != "":
		msg = *jr.Failure
	case jr.Skipped != nil && *jr.Skipped != "":
		msg = *jr.Skipped
	case jr.Error != nil && *jr.Error != "":
		msg = *jr.Error
	case jr.Output != nil && *jr.Output != "":
		msg = *jr.Output
	}
	l := len(msg)
	if max == 0 || l <= max {
		return msg
	}
	h := max / 2
	return msg[:h] + "..." + msg[l-h-1:]
}

func unmarshalXML(buf []byte, i interface{}) error {
	reader := bytes.NewReader(buf)
	dec := xml.NewDecoder(reader)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch charset {
		case "UTF-8", "utf8", "":
			// utf8 is not recognized by golang, but our coalesce.py writes a utf8 doc, which python accepts.
			return input, nil
		default:
			return nil, fmt.Errorf("unknown charset: %s", charset)
		}
	}
	return dec.Decode(i)
}

func Parse(buf []byte) (Suites, error) {
	var suites Suites
	// Try to parse it as a <testsuites/> object
	err := unmarshalXML(buf, &suites)
	if err != nil {
		// Maybe it is a <testsuite/> object instead
		suites.Suites = append([]Suite(nil), Suite{})
		ie := unmarshalXML(buf, &suites.Suites[0])
		if ie != nil {
			// Nope, it just doesn't parse
			return suites, fmt.Errorf("not valid testsuites: %v nor testsuite: %v", err, ie)
		}
	}
	return suites, nil
}
