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
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/glog"
	git "gopkg.in/src-d/go-git.v4"
)

var (
	gitRegexp = regexp.MustCompile(`^\.git$`)
)

// GitFileURL returns the URL of a file stored in a Git repository.
// It uses the URL of the specified Git remote repository to construct this URL.
// It supports repositories hosted on github.com, bitbucket.org and googlesource.com.
func GitFileURL(filePath string, remote string) (*url.URL, error) {
	dotGitPath, err := findUpwards(filepath.Dir(filePath), gitRegexp, srcDirRegexps)
	if err != nil {
		return nil, err
	}
	relFilePath, err := filepath.Rel(filepath.Dir(dotGitPath), filePath)
	if err != nil {
		return nil, err
	}
	repoURL, err := gitRemoteURL(dotGitPath, remote)
	if err != nil {
		return nil, err
	}
	repoURL.Host = strings.TrimSuffix(repoURL.Host, ".")
	repoURL.Path = strings.TrimSuffix(repoURL.Path, ".git")
	switch repoURL.Host {
	case "github.com":
		repoURL.Path = path.Join(repoURL.Path, "blob/master/", filepath.ToSlash(relFilePath))
	case "bitbucket.org":
		repoURL.Path = path.Join(repoURL.Path, "src/master/", filepath.ToSlash(relFilePath))
	case "go.googlesource.com", "code.googlesource.com":
		repoURL.Path = path.Join(repoURL.Path, "+/refs/heads/master/", filepath.ToSlash(relFilePath))
	default:
		return nil, fmt.Errorf("unrecognised Git repository host: %q", repoURL)
	}
	return repoURL, nil
}

func gitRemoteURL(repoPath string, remoteName string) (*url.URL, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	remote, err := repo.Remote(remoteName)
	if err != nil {
		return nil, err
	}
	for _, urlStr := range remote.Config().URLs {
		u, err := url.Parse(urlStr)
		if err != nil {
			glog.Warningf("Error parsing %q as URL from remote %q in Git repo at %q: %s", urlStr, remoteName, repoPath, err)
			continue
		}
		return u, nil
	}
	return nil, fmt.Errorf("Git remote %q does not have a valid URL", remoteName)
}
