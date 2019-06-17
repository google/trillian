#!/bin/bash -ex
#
# Instantiate a SoftHSM token, then run tests, including the benchmark. Provide
# flags allowing the benchmark to use the SoftHSM token.
#
# You can use a different PKCS#11 module by setting the MODULE environment
# variable, though you will still need the softhsm or softhsm2-util command
# line tool to initialize the token(s) and load the key(s). For instance:
#
# export MODULE=/usr/local/lib/libpkcs11-proxy.so PKCS11_PROXY_SOCKET=tcp://hsm.example.com:5657
# bash test.sh
#
# You can also override the number of sessions by setting the SESSIONS variable.

if [ -r /proc/brcm_monitor0 ]; then
  echo "The /proc/brcm_monitor0 file has open permissions. Please run"
  echo " # chmod 600 /proc/brcm_monitor0"
  echo "as root to avoid crashing the system."
  echo https://bugs.launchpad.net/ubuntu/+source/bcmwl/+bug/1450825
  exit 2
fi

cd $(dirname $0)
DIR=$(mktemp -d -t softhXXXX)

# Travis doesn't currently offer softhsm2 (it's not in Ubuntu Trusty), so we run
# softhsm2 if it's available (e.g., locally), and softhsm otherwise (e.g. in
# Travis). ECDSA is only supported in SoftHSMv2 so we skip the ECDSA benchmark
# when it's not available.
if $(type softhsm2-util 2>/dev/null >&2) ; then
  MODULE=${MODULE:-/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so}
  export SOFTHSM2_CONF=${DIR}/softhsm.conf
  echo directories.tokendir = ${DIR} > ${SOFTHSM2_CONF}
  softhsm2-util --module ${MODULE} --slot 0 --init-token --label silly_signer --pin 1234 --so-pin 1234
  softhsm2-util --module ${MODULE} --slot 0 --import testdata/silly_signer.key --label silly_signer_key --pin 1234 --id F00D
  softhsm2-util --module ${MODULE} --slot 1 --init-token --label entropic_ecdsa --pin 1234 --so-pin 1234
  softhsm2-util --module ${MODULE} --slot 1 --import testdata/entropic_ecdsa.key --label entropic_ecdsa_key --pin 1234 --id C0FFEE
else
  MODULE=${MODULE:-/usr/lib/libsofthsm.so}
  export SOFTHSM_CONF=${DIR}/softhsm.conf
  SLOT=0
  echo ${SLOT}:${DIR}/softhsm-slot${SLOT}.db > ${SOFTHSM_CONF}
  softhsm --module ${MODULE} --slot ${SLOT} --init-token --label silly_signer --pin 1234 --so-pin 1234
  softhsm --module ${MODULE} --slot ${SLOT} --import testdata/silly_signer.key --label silly_signer_key --pin 1234 --id F00D
fi

go test github.com/letsencrypt/pkcs11key

# Run the benchmark. Arguments: $1: token label, $2: certificate filename
function bench {
  go test github.com/letsencrypt/pkcs11key -module ${MODULE} \
    -test.run xxxNONExxx \
    -pin 1234 -tokenLabel ${1} \
    -cert ${2} \
    -test.bench Bench \
    -benchtime 10s \
    -sessions ${SESSIONS:-2};
}

bench silly_signer testdata/silly_signer.pem

if [ -n "${SOFTHSM2_CONF}" ] ; then
  bench entropic_ecdsa testdata/entropic_ecdsa.pem
fi

rm -r ${DIR}
