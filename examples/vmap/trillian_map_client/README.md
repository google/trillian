# Trillian Map Server client example

## Start a Trillian Map Server
```bash
RPCS=8095
HTTP=8096
go run github.com/google/trillian/server/trillian_map_server \
--logtostderr \
--rpc_endpoint=":${RPCS}" \
--http_endpoint=":${HTTP}"
```

The Trillian Map Server assumes you have a MySQL instance running and configured, see:

https://github.com/google/trillian#mysql-setup

You may monitor the server's Prometheus metric exporter on:

`http://${MAP_SERVER}:${HTTP}/metrics`

## Generate a Tree
```bash
MAPID=$(go run github.com/google/trillian/cmd/createtree \
  --admin_server=":${RPCS}" \
  --tree_type=MAP \
  --hash_strategy=TEST_MAP_HASHER \ # Optional but this value is required
) && echo ${MAPID}
```

## Run the sample

```bash
go run $GOPATH/google/trillian/examples/vmap/trillian_map_client/
--logtostderr \
--server=":${RPCS} \
--map_id=${MAPID}
```