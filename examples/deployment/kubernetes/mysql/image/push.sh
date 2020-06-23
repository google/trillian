#!/bin/bash -e

# Copyright 2017 Google LLC. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

tag="us.gcr.io/trillian-test/galera:experimental"

usage=$(cat <<EOF
Usage: $(basename $0) [-t tag]

Builds and pushes the image in this directory to a container repository.
By default, the image will be tagged with ${tag} and
pushed to the Google Cloud container repository.
EOF
)

while getopts "ht:" opt; do
  case $opt in
    h)
      echo "$usage"; exit 0
      ;;
    t)
      tag=$OPTARG
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >/dev/stderr; exit 1
      ;;
    :)
      echo "Option -$OPTARG requires an argument." >/dev/stderr; exit 1
      ;;
  esac
done

gcloud auth configure-docker
docker build -t ${tag} .
docker push ${tag}

