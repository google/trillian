#!/bin/bash -ex
#
# Instantiate a SoftHSM token, then run tests, including the benchmark. Provide
# flags allowing the benchmark to use the SoftHSM token.
#

if [ -r /proc/brcm_monitor0 ]; then
  echo "The /proc/brcm_monitor0 file has open permissions. Please run"
  echo " # chmod 600 /proc/brcm_monitor0"
  echo "as root to avoid crashing the system."
  echo https://bugs.launchpad.net/ubuntu/+source/bcmwl/+bug/1450825
  exit 2
fi

DIR=$(mktemp -d -t softhXXXX)

# Travis doesn't currently offer softhsm2 (it's not in Ubuntu Trusty), so we run
# softhsm2 if it's available (e.g., locally), and softhsm otherwise (e.g. in
# Travis). ECDSA is only supported in SoftHSMv2 so we skip the ECDSA benchmark
# when it's not available.
if $(type softhsm2-util 2>/dev/null >&2) ; then
  MODULE=/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so
  export SOFTHSM2_CONF=${DIR}/softhsm.conf
  echo directories.tokendir = ${DIR} > ${SOFTHSM2_CONF}
  softhsm2-util --slot 0 --init-token --label silly_signer --pin 1234 --so-pin 5678
  softhsm2-util --slot 0 --import testdata/silly_signer.key --label silly_signer_key --pin 1234 --id F00D
  softhsm2-util --slot 1 --init-token --label entropic_ecdsa --pin 1234 --so-pin 5678
  softhsm2-util --slot 1 --import testdata/entropic_ecdsa.key --label entropic_ecdsa_key --pin 1234 --id C0FFEE
else
  MODULE=/usr/lib/softhsm/libsofthsm.so
  export SOFTHSM_CONF=${DIR}/softhsm.conf
  SLOT=0
  echo ${SLOT}:${DIR}/softhsm-slot${SLOT}.db > ${SOFTHSM_CONF}
  softhsm --slot ${SLOT} --init-token --label silly_signer --pin 1234 --so-pin 5678
  softhsm --slot ${SLOT} --import testdata/silly_signer.key --label silly_signer_key --pin 1234 --id F00D
fi

go test github.com/letsencrypt/pkcs11key

go test github.com/letsencrypt/pkcs11key -module ${MODULE} \
  -test.run xxxNONExxx \
  -pin 1234 -tokenLabel "silly_signer" \
  -cert testdata/silly_signer.pem \
  -test.bench Bench \
  -sessions 2

if [ -n "${SOFTHSM2_CONF}" ] ; then
  go test github.com/letsencrypt/pkcs11key -module ${MODULE} \
    -test.run xxxNONExxx \
    -pin 1234 -tokenLabel "entropic_ecdsa" \
    -cert testdata/entropic_ecdsa.pem \
    -test.bench Bench \
    -sessions 2
fi

rm -r ${DIR}
