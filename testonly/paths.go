package testonly

import (
	"path"
	"path/filepath"
	"runtime"
)

// RelativeToPackage returns the input path p as an absolute path, resolved relative to the caller's package.
// The working directory for Go tests is the dir of the test file. Using "plain" relative paths in test
// utilities is, therefore, brittle, as the directory structure may change depending on where the tests are placed.
func RelativeToPackage(p string) string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("cannot get caller information")
	}

	absPath, err := filepath.Abs(filepath.Join(path.Dir(file), p))
	if err != nil {
		panic(err)
	}
	return absPath
}
