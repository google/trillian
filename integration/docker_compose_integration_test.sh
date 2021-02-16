#!/bin/bash

readonly DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null && pwd)"

# Should be run from the root Trillian directory.
docker_compose_up() {
  local http_addr="$1"

  # See: https://docs.docker.com/compose/extends/#multiple-compose-files.
  docker-compose -f examples/deployment/docker-compose.yml \
    -f integration/cloudbuild/docker-compose.network.yml up --build -d

  # Wait until /healthz returns HTTP 200 and the text "ok", or fail after 30
  # seconds. That should be long enough for the server to start. Since wget
  # doesn't retry DNS failures, wrap this in a loop, so that Docker containers
  # have time to join the network and update the DNS.
  for i in {1..10} ; do
    health=$(wget --retry-connrefused --timeout 30 --output-document - \
      "http://${http_addr}/healthz")
    if [[ $? = 0 ]]; then
      break
    fi
    sleep 5
  done
  health_exitcode=$?
  if [[ ${health_exitcode} = 0 ]]; then
    echo "Health: ${health}"
  fi

  if [[ ${health_exitcode} != 0 || "${health}" != "ok" ]]; then
    return 1
  fi
  return 0
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  # Change to the root Trillian directory.
  cd "$DIR/.."

  if docker_compose_up "deployment_trillian-log-server_1:8091" && \
     integration/log_integration_test.sh "deployment_trillian-log-server_1:8090"; then
    docker-compose -f examples/deployment/docker-compose.yml down
  else
    echo "Docker logs:"
    docker-compose -f examples/deployment/docker-compose.yml logs
    docker-compose -f examples/deployment/docker-compose.yml down
    exit 1
  fi
fi
