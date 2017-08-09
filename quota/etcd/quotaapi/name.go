// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quotaapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	collectionTrees = "trees"
	wildcard        = "-"
)

var (
	// globalRE is a regex for quotas/global/ name filters.
	// Wildcards ("-") are not allowed to replace "global". They could be allowed, but it's a query
	// that doesn't make much sense, as if this format is used it has to be a global quota query.
	globalRE *regexp.Regexp

	// treeUsersRE is a broader regex for all trees/ and users/ name filters.
	// It doesn't guard against the following specific situations:
	// 1. quotas/trees/<id>/<kind>/config where id is not an int64
	// 2. quotas/-/<id>/<kind>/config where id != "-", which is a possibly ambiguous query. It could
	//    be allowed, but since the ID being queried is already known, so is the collection, so it
	//    doesn't make much sense.
	treesUsersRE *regexp.Regexp
)

func init() {
	var err error
	globalRE, err = regexp.Compile("^quotas/global/(-|read|write)/config$")
	if err != nil {
		panic(fmt.Sprintf("globalRE: %v", err))
	}
	treesUsersRE, err = regexp.Compile("^quotas/(-|trees|users)/[^/]+/(-|read|write)/config$")
	if err != nil {
		panic(fmt.Sprintf("treesUsersRE: %v", err))
	}
}

// nameFilter represents a config name filter, as used by ListConfigs.
//
// A name filter is internally represented as the segments of the name, ie, the result of
// strings.Split(name, "/").
//
// A few examples are:
// * ["quotas", "global", "read", "configs"]
// * ["quotas", "trees", "12345", "write", "config"]
type nameFilter []string

func newNameFilter(name string) (nameFilter, error) {
	if !globalRE.MatchString(name) && !treesUsersRE.MatchString(name) {
		return nil, fmt.Errorf("invalid name filter: %q", name)
	}

	nf := strings.Split(string(name), "/")

	// Guard against some ambiguous / incorrect wildcards that the regexes won't protect against
	switch collection := nf[1]; collection {
	case collectionTrees:
		id := nf[2]
		if id == wildcard {
			break
		}
		// treeID must be an int64
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid name filter: %q, ID %q is not a valid 64-bit integer", name, id)
		}
	case wildcard:
		id := nf[2]
		if id != wildcard {
			return nil, fmt.Errorf("invalid name filter: %q, ambiguous ID %q received", name, id)
		}
	}
	return nf, nil
}

func (nf nameFilter) matches(path string) bool {
	segments := strings.Split(path, "/")

	l := len(nf)
	if l != len(segments) {
		return false
	}

	// Skip first and last tokens (they're always "quotas" and "config").
	for i := 1; i < l-1; i++ {
		if nf[i] != wildcard && nf[i] != segments[i] {
			return false
		}
	}

	return true
}
