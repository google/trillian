#!/bin/bash

set -e

download() {
  COMMIT=8f52a814ef0cc70820b87fbf888273f3aa7f5a9b
  URL=https://raw.githubusercontent.com/vishnubob/wait-for-it/${COMMIT}/wait-for-it.sh
  curl -sO $URL
  chmod a+x wait-for-it.sh
}

download
sha256sum --check <( echo "c238c56e2a81b3c97375571eb4f58a0e75cdb4cd957f5802f733ac50621e776a wait-for-it.sh" )
