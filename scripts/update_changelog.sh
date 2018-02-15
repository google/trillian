#! /bin/bash
#
# Run this script from the top level directory of the trillian repo e.g.
# with scripts/update_changelog.sh.
#
# GOPATH must be set.

set +e
d=${GOPATH[0]}

# Get and build the correct branch that includes markdown output
# TODO(Martin2112): replace with upstream repo if/when aktau/github-release#81 is merged
go install github.com/Martin2112/github-release

# Generate the changelog
${d}/bin/github-release info -r trillian -u google --markdown > CHANGELOG.md

