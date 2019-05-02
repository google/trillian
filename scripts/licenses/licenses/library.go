// Copyright 2019 Google Inc. All Rights Reserved.
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

package licenses

import (
	"fmt"
	"go/build"
	"regexp"
	"sort"
	"sync"

	"github.com/golang/glog"
)

var (
	licenseRegexp = regexp.MustCompile(`^LICENSE(\.(txt|md))?$`)
	srcDirRegexps = func() []*regexp.Regexp {
		var rs []*regexp.Regexp
		for _, s := range build.Default.SrcDirs() {
			rs = append(rs, regexp.MustCompile("^"+regexp.QuoteMeta(s)+"$"))
		}
		return rs
	}()
	vendorRegexp = regexp.MustCompile(`.+/vendor(/)?$`)

	pkgCache sync.Map
)

// Library is a collection of packages covered by the same license file.
type Library struct {
	Packages    []*build.Package
	LicensePath string
}

// Libraries returns the collection of libraries used by this package, directly or transitively.
// A library is a collection of one or more packages covered by the same license file.
// Packages not covered by a license will be returned as individual libraries.
// Packages covered by the same license as pkg will be ignored.
// Standard library packages will be ignored.
func Libraries(ctx *build.Context, pkg *build.Package) ([]*Library, error) {
	pkgLicense, err := Find(pkg)
	if err != nil {
		return nil, err
	}
	deps := make(map[string]*build.Package)
	if err := dependencies(ctx, pkg, deps); err != nil {
		return nil, err
	}
	pkgsByLicense := make(map[string][]*build.Package)
	for _, d := range deps {
		if isStdLib(d) {
			// No license requirements for the Go standard library.
			continue
		}
		licensePath, err := Find(d)
		if err != nil {
			glog.Errorf("Failed to find license for %s: %v", d.ImportPath, err)
		}
		if licensePath == pkgLicense {
			// Skip dependencies that share a license file with pkg.
			// These are not external libraries.
			continue
		}
		pkgsByLicense[licensePath] = append(pkgsByLicense[licensePath], d)
	}
	var libraries []*Library
	for licensePath, pkgs := range pkgsByLicense {
		if licensePath == "" {
			// No license for these packages - return each one as a separate library.
			for _, pkg := range pkgs {
				libraries = append(libraries, &Library{
					Packages: []*build.Package{pkg},
				})
			}
			continue
		}
		libraries = append(libraries, &Library{
			LicensePath: licensePath,
			Packages:    pkgs,
		})
	}
	return libraries, nil
}

// Name is the common prefix of the import paths for all of the packages in this library.
func (l *Library) Name() string {
	if len(l.Packages) == 0 {
		return ""
	}
	if len(l.Packages) == 1 {
		return l.Packages[0].ImportPath
	}
	var importPaths []string
	for _, pkg := range l.Packages {
		importPaths = append(importPaths, pkg.ImportPath)
	}
	sort.Strings(importPaths)
	min, max := importPaths[0], importPaths[len(importPaths)-1]
	lastSlashIndex := 0
	for i := 0; i < len(min) && i < len(max); i++ {
		if min[i] != max[i] {
			return min[:lastSlashIndex]
		}
		if min[i] == '/' {
			lastSlashIndex = i
		}
	}
	return min
}

func (l *Library) String() string {
	return l.Name()
}

// ImportPackage returns information about the package identified by the given import path.
// If there is a "vendor" directory in workingDir, packages in that directory will take precedence
// over packages with the same import path found elsewhere.
func importPackage(ctx *build.Context, importPath string, workingDir string) (*build.Package, error) {
	cacheKey := workingDir + ":" + importPath
	if pkg, ok := pkgCache.Load(cacheKey); ok {
		return pkg.(*build.Package), nil
	}

	pkg, err := ctx.Import(importPath, workingDir, 0)
	if err != nil {
		return nil, err
	}

	pkgCache.Store(cacheKey, pkg)
	return pkg, nil
}

// IsStdLib returns true if this package is part of the Go standard library.
func isStdLib(pkg *build.Package) bool {
	return pkg.Root == build.Default.GOROOT
}

// Dependencies finds the Go packages used by this package, directly or transitively.
// They are added to the provided deps map.
func dependencies(ctx *build.Context, pkg *build.Package, deps map[string]*build.Package) error {
	for _, imp := range pkg.Imports {
		if imp == "C" {
			return fmt.Errorf("%s has a dependency on C code, which cannot be inspected for further dependencies", pkg.ImportPath)
		}
		if _, ok := deps[imp]; !ok {
			impPkg, err := importPackage(ctx, imp, pkg.Dir)
			if err != nil {
				return fmt.Errorf("%s -> %v", pkg.ImportPath, err)
			}
			deps[imp] = impPkg
			if isStdLib(impPkg) {
				// Don't delve into standard library dependencies - that'll just lead to dependencies on other parts of the standard library,
				// which isn't of interest (no license requirements for the standard library).
				continue
			}
			// Collect transitive dependencies
			if err := dependencies(ctx, impPkg, deps); err != nil {
				return fmt.Errorf("%s -> %v", pkg.ImportPath, err)
			}
		}
	}
	return nil
}
