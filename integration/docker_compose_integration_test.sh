#!/bin/bash

readonly DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null && pwd)"

# Should be run from the root Trillian directory.
docker_compose_up() {
  local http_addr="$1"

  docker-compose -f examples/deployment/docker-compose.yml up --build -d

  # Wait until /healthz returns HTTP 200 and the text "ok", or fail after 60
  # seconds. That should be long enough for the server to start.
  health=$(wget --retry-connrefused --timeout 60 --output-document - \
    "http://${http_addr}/healthz")
  health_exitcode=$?
  if [[ ${health_exitcode} = 0 ]]; then
    echo "Health: ${health}"
  fi

  if [[ ${health_exitcode} != 0 && "${health}" != "ok" ]]; then
    return 1
  fi
  return 0
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  # Change to the root Trillian directory.
  cd "$DIR/.."

  if docker_compose_up "127.0.0.1:8091" && integration/log_integration_test.sh "127.0.0.1:8090"; then
    docker-compose -f examples/deployment/docker-compose.yml down
  else
    echo "Docker logs:"
    docker-compose -f examples/deployment/docker-compose.yml logs
    docker-compose -f examples/deployment/docker-compose.yml down
    exit 1
  fi
fi
