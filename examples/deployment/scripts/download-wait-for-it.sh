#!/bin/bash

set -e

download() {
  COMMIT=8f52a814ef0cc70820b87fbf888273f3aa7f5a9b
  URL=https://raw.githubusercontent.com/vishnubob/wait-for-it/${COMMIT}/wait-for-it.sh
  curl -sO $URL
  chmod a+x wait-for-it.sh
}

download
echo "ffe253ce564df22adbcf9c799e251ca0  wait-for-it.sh" > file.md5
md5sum -c file.md5
