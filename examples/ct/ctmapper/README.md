# Example CT Mapper

This is an example of a process which maps from a verifiable Log to a
verifiable Map.
It scans an RFC6962 CT Log server for certificate and precertificates,
and adds entries to a Verifiable Map whose keys are SHA256(domainName), and
whose values are a protobuf of indicies in the log where precerts/certs exist
which have that domain in their subject/SAN fields.

## Running the example

```bash
# Ensure you have your MySQL DB set up correctly, with tables created by the
# contents of storage/mysql/schema/storage.sql
yes | scripts/resetdb.sh

go build ./cmd/trillian_map_server
go build ./examples/ct/ctmapper/mapper
go build ./examples/ct/ctmapper/lookup

# in one terminal:
./trillian_map_server --logtostderr

# in another (leaving the trillian_map_server running):
go build ./cmd/createtree/
tree_id=$(./createtree \
    --admin_server=localhost:8090 \
    --hash_strategy=TEST_MAP_HASHER \
    --tree_type=MAP)
echo "Created map with ID ${tree_id}"

./mapper \
    --logtostderr \
    --log_batch_size=10 \
    --map_id=${tree_id} \
    --map_server=localhost:8090 \
    --source=http://ct.googleapis.com/pilot
```

You should then be able to look up domains in the map like so:

```bash
./lookup \
    --logtostderr \
    --map_id=${tree_id} \
    --map_server=localhost:8090 \
    mail.google.com www.langeoog.de  # etc. etc.
```
