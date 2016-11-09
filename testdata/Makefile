all: pubkeys ca

# The following private keys are never regenerated.
SERVER_PRIVKEYS=ct-http-server.privkey.pem log-rpc-server.privkey.pem map-rpc-server.privkey.pem

# Corresponding passwords:
CT_HTTP_PWD=dirk
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
ct-http-server.pubkey.pem: ct-http-server.privkey.pem
	openssl ec -in $< -pubout -out $@ -passin pass:$(CT_HTTP_PWD)


# We use a fake CA as a trust root for CT tests. This is its private key.
CA_PRIVKEY=fake-ca.privkey.pem
CA_PWD=gently

# We also have an intermediate CA, with private key:
INT_CA_PRIVKEY=int-ca.privkey.pem
INT_CA_PWD=babelfish

ca: fake-ca.cert int-ca.cert

fake-ca.cert: fake-ca.privkey.pem fake-ca.cfg
	openssl req -new -x509 -config fake-ca.cfg -set_serial 0x0406cafe -days 3650 -extensions v3_ca -inform pem -key fake-ca.privkey.pem -passin pass:$(CA_PWD) -out $@
show-ca: fake-ca.cert
	openssl x509 -inform pem -in $< -text -noout

int-ca.csr.pem: int-ca.privkey.pem int-ca.cfg
	openssl req -new -sha256 -config int-ca.cfg -key int-ca.privkey.pem -passin pass:$(INT_CA_PWD) -out $@
show-int-csr: int-ca.csr.pem
	openssl req -in $< -text -noout
int-ca.cert: int-ca.csr.pem fake-ca.privkey.pem fake-ca.cert
	openssl x509 -req -in int-ca.csr.pem -sha256 -extfile fake-ca.cfg -extensions v3_ca -CA fake-ca.cert -CAkey fake-ca.privkey.pem  -passin pass:gently -set_serial 0x42424242 -days 3600 -out $@
show-int-ca: int-ca.cert
	openssl x509 -inform pem -in $< -text -noout

# clean removes things that regenerate exactly the same.
clean:
	rm -f $(SERVER_PUBKEYS)
# distclean removes things that regenerate with changes (e.g. timestamped, randomized).
distclean: clean
	rm -f $(SERVER_PUBKEYS) fake-ca.cert int-ca.cert int-ca.csr.pem

# The newkey target creates a fresh private key; should never be needed.
newkey: fresh.privkey.pem
fresh.privkey.pem:
	openssl ecparam -genkey -name prime256v1 -noout -out $@.unencrypted
	openssl ec -in $@.unencrypted -out $@ -des  # Prompts for password
	rm -f $@.unencrypted

