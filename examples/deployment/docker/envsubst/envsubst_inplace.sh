#!/bin/sh -e

for f in "$@"; do
  tmpfile=$(mktemp)
  envsubst < "$f" > "$tmpfile"
  mv "$tmpfile" "$f"
done

