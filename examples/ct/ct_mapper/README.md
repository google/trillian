# Example CT Mapper

This is an example of a process which maps from a verifiable Log to a
verifiable Map.
It scans an RFC6962 CT Log server for certificate and precertificates,
and adds entries to a Verifiable Map whose keys are SHA256(domainName), and
whose values are a protobuf of indicies in the log where precerts/certs exist
which have that domain in their subject/SAN fields.

To run:

```bash
# Ensure you have your MySQL DB set up correctly, with tables created by the contents of storage/mysql/storage.sql
# Insert a new entry into the Trees table to provision a Map:
mysql -u root -p test -e "source storage/mysql/drop_storage.sql;  source storage/mysql/storage.sql; insert into Trees values(1, 1, 'MAP', 'SHA256', 'SHA256', false);"

go build ./server/vmap/trillian_map_server
go build ./examples/ct/ct_mapper

# in one terminal:
./trillian_map_server --logtostderr --private_key_password=towel --private_key_file=testdata/trillian-map-server-key.pem

# in another (leaving the trillian_map_server running):
./ct_mapper -source http://ct.googleapis.com/testtube -map_id=1 -map_server=localhost:8091 --logtostderr
```
