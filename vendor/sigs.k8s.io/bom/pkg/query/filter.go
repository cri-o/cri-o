/*
Copyright 2022 The Kubernetes Authors.

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

package query

import (
	"fmt"
	"strings"

	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"

	"sigs.k8s.io/bom/pkg/spdx"
)

type Filter interface {
	Apply(map[string]spdx.Object) (map[string]spdx.Object, error)
}

type FilterResults struct {
	Objects map[string]spdx.Object
	Error   error
}

func (fr *FilterResults) Apply(filter Filter) *FilterResults {
	// If the filter results have an error. Stop here
	if fr.Error != nil {
		return fr
	}

	newObjSet, err := filter.Apply(fr.Objects)
	if err != nil {
		fr.Error = err
		return fr
	}
	fr.Objects = newObjSet
	return fr
}

type DepthFilter struct {
	TargetDepth int
}

func (f *DepthFilter) Apply(objects map[string]spdx.Object) (map[string]spdx.Object, error) {
	// Perform filter
	return searchDepth(objects, 0, uint(f.TargetDepth)), nil
}

func searchDepth(objectSet map[string]spdx.Object, currentDepth, targetDepth uint) map[string]spdx.Object {
	// If we are at target depth, we are done
	if targetDepth == currentDepth {
		return objectSet
	}

	res := map[string]spdx.Object{}
	for _, o := range objectSet {
		// If not, cycle the objects relationships to search further down
		for _, r := range *o.GetRelationships() {
			if r.Peer != nil && r.Peer.SPDXID() != "" {
				res[r.Peer.SPDXID()] = r.Peer
			}
		}
	}
	if targetDepth == currentDepth {
		return res
	}

	return searchDepth(res, currentDepth+1, targetDepth)
}

// AllFilter matches everything
type AllFilter struct{}

func (f *AllFilter) Apply(objects map[string]spdx.Object) (map[string]spdx.Object, error) {
	cycler := ObjectCycler{}
	return cycler.CycleFull(objects, func(spdx.Object) bool { return true }), nil
}

type NameFilter struct {
	Pattern string
}

func (f *NameFilter) Apply(objects map[string]spdx.Object) (map[string]spdx.Object, error) {
	// Perform filter
	cycler := ObjectCycler{}
	return cycler.Cycle(objects, func(o spdx.Object) bool {
		if _, ok := o.(*spdx.File); ok {
			return strings.Contains(o.(*spdx.File).FileName, f.Pattern)
		}
		if _, ok := o.(*spdx.Package); ok {
			return strings.Contains(o.(*spdx.Package).Name, f.Pattern)
		}
		return false
	}), nil
}

type PurlFilter struct {
	Pattern string
}

func (f *PurlFilter) Apply(objects map[string]spdx.Object) (map[string]spdx.Object, error) {
	patternPurl, err := purl.FromString(f.Pattern)
	if err != nil {
		return nil, fmt.Errorf("parsing purl: %w", err)
	}

	logrus.Infof("Purl: %s", patternPurl)
	if patternPurl.Type == "" {
		patternPurl.Type = "*"
	}

	if patternPurl.Name == "" {
		patternPurl.Name = "*"
	}

	if patternPurl.Version == "" {
		patternPurl.Version = "*"
	}

	if patternPurl.Namespace == "" {
		patternPurl.Namespace = "*"
	}
	cycler := ObjectCycler{}
	return cycler.Cycle(objects, func(o spdx.Object) bool {
		p, ok := o.(*spdx.Package)
		if !ok {
			logrus.Info("No package")
			return false
		}
		if p.Purl() == nil {
			return false
		}
		return p.PurlMatches(&patternPurl)
	}), nil
}

type MatcherFunction func(spdx.Object) bool

type ObjectCycler struct{}

func (cycler *ObjectCycler) Cycle(objects map[string]spdx.Object, fn MatcherFunction) map[string]spdx.Object {
	return doRecursion(objects, fn, &map[string]struct{}{})
}

func (cycler *ObjectCycler) CycleFull(objects map[string]spdx.Object, fn MatcherFunction) map[string]spdx.Object {
	return doFullRecursion(objects, fn, &map[string]struct{}{})
}

// Recursion will traverse the SBOM graph and return the element that
// matches the query without continuing down its relationships
func doRecursion(
	//nolint:gocritic // seen is passed recursively
	objects map[string]spdx.Object, fn MatcherFunction, seen *map[string]struct{},
) map[string]spdx.Object {
	newSet := map[string]spdx.Object{}
	for _, o := range objects {
		if o.SPDXID() == "" {
			continue
		}
		if _, ok := (*seen)[o.SPDXID()]; ok {
			continue
		}
		(*seen)[o.SPDXID()] = struct{}{}

		if fn(o) {
			newSet[o.SPDXID()] = o
			continue
		}

		// do a new recursion on the related objects
		subSet := map[string]spdx.Object{}
		for _, r := range *o.GetRelationships() {
			if r.Peer != nil && r.Peer.SPDXID() != "" {
				// We only recurse on the first match of each object
				if _, ok := subSet[r.Peer.SPDXID()]; !ok {
					subSet[r.Peer.SPDXID()] = r.Peer
				}
			}
		}
		filteredSet := doRecursion(subSet, fn, seen)
		for _, o := range filteredSet {
			newSet[o.SPDXID()] = o
		}
	}
	return newSet
}

// doFullRecursion will probe all objects in the sbom, when matching an
// object, it will continue traversing its relationships returning all
// matching objects in a flat array
func doFullRecursion(
	//nolint:gocritic // seen is passed recursively
	objects map[string]spdx.Object, fn MatcherFunction, seen *map[string]struct{},
) map[string]spdx.Object {
	newSet := map[string]spdx.Object{}
	for _, o := range objects {
		if o.SPDXID() == "" {
			continue
		}
		if _, ok := (*seen)[o.SPDXID()]; ok {
			continue
		}
		(*seen)[o.SPDXID()] = struct{}{}

		if fn(o) {
			newSet[o.SPDXID()] = o
		}

		// do a new recursion on the related objects
		subSet := map[string]spdx.Object{}
		for _, r := range *o.GetRelationships() {
			if r.Peer != nil && r.Peer.SPDXID() != "" {
				// We only recurse on the first match of each object
				if _, ok := subSet[r.Peer.SPDXID()]; !ok {
					subSet[r.Peer.SPDXID()] = r.Peer
				}
			}
		}
		filteredSet := doFullRecursion(subSet, fn, seen)
		for _, o := range filteredSet {
			newSet[o.SPDXID()] = o
		}
	}
	return newSet
}
