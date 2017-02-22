#!/bin/sh
set -eu

if [ $# -ne 1 ]; then
    echo "Usage: $0 <treeid>"
    exit 1
fi

"$(dirname "$0")"/createtree.sh $1 'LOG'
