#! /bin/bash
#
# Run this script from the top level directory of the trillian repo e.g.
# with scripts/update_changelog.sh.
#
# GOPATH must be set.

set +e
d=${GOPATH[0]}
PATH=$PATH:${d}/bin

# Get and build the correct branch that includes markdown output
go get -d -u github.com/Martin2112/github-release
pushd $d/src/github.com/Martin2112/github-release
git fetch origin add_changelog_output
git checkout add_changelog_output
go install github.com/Martin2112/github-release

# Generate the changelog
github-release info -r trillian -u google --markdown > CHANGELOG.md

