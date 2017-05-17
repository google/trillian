all: pubkeys

# The following private keys are never regenerated.
SERVER_PRIVKEYS=log-rpc-server.privkey.pem map-rpc-server.privkey.pem

# Corresponding passwords:
LOG_RPC_PWD=towel
MAP_RPC_PWD=towel

# Server public keys are derived from the corresponding private keys.
SERVER_PUBKEYS=$(subst .privkey,.pubkey,$(SERVER_PRIVKEYS))

# Build public keys from private keys
pubkeys: $(SERVER_PUBKEYS)
map-rpc-server.pubkey.pem: map-rpc-server.privkey.pem
	openssl ec -in $< -pubout -out $@ -passin pass:$(MAP_RPC_PWD)
log-rpc-server.pubkey.pem: log-rpc-server.privkey.pem
	openssl ec -in $< -pubout -out $@ -passin pass:$(LOG_RPC_PWD)

clean:
	rm -f $(SERVER_PUBKEYS)

# The newkey target creates a fresh private key; should never be needed.
newkey: fresh.privkey.pem
fresh.privkey.pem:
	openssl ecparam -genkey -name prime256v1 -noout -out $@.unencrypted
	openssl ec -in $@.unencrypted -out $@ -des  # Prompts for password
	rm -f $@.unencrypted

