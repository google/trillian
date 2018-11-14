#!/bin/bash -e

readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

# Pick a random port between 2000 - 34767. Will fail if unlucky and the port is in use.
readonly port=$((${RANDOM} + 2000))
kubectl port-forward "galera-0" "${port}:3306" >/dev/null &
proxy_pid=$!

options_file=$(mktemp)
chmod 0600 "${options_file}"
echo > ${options_file} <<EOF
[client]
host=localhost
port=${port}
user=root
password="$(kubectl get secrets mysql-credentials --template '{{index .data "root-password"}}' | base64 -d)"
EOF

"${TRILLIAN_PATH}/scripts/resetdb.sh" --defaults-extra-file="${options_file}"

rm "${options_file}"

kill "${proxy_pid}"
wait "${proxy_pid}" 2>/dev/null

