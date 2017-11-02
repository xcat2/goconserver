#!/bin/bash

if [ -z "$CONSOLESERVER_DIR" ]; then
  CONSOLESERVER_DIR=/etc/goconserver
fi
if [ -z "$1" ]; then
  echo "Usage: $0 <CA name>"
  exit 1
fi
CNA="$*"
CA_DIR=$CONSOLESERVER_DIR/ca

if [ -e $CA_DIR ]; then
  echo -n "Existing xCAT certificate authority detected at $XCATDIR/ca, delete? (y/n):"
  read ANSWER
  if [ $ANSWER != 'y' ]; then
    echo "Aborting install at user request"
    exit 0;
  fi
  rm -rf $CA_DIR
  mkdir -p $CA_DIR
else
  mkdir -p $CA_DIR
fi

sed -e "s@##CERT_DIR##@$CA_DIR@" openssl.cnf.tmpl > $CA_DIR/openssl.cnf

mkdir -p $CA_DIR/crl $CA_DIR/certs $CA_DIR/private
chmod go-rwx $CA_DIR/private
echo '01' > $CA_DIR/serial
touch $CA_DIR/index
cd $CA_DIR
openssl genrsa -out private/ca-key.pem 2048
chmod 600 private/ca-key.pem
openssl req -new -key private/ca-key.pem -config openssl.cnf -out ca-req.csr -subj /CN="$CNA" -outform PEM
openssl ca -selfsign -keyfile private/ca-key.pem -in ca-req.csr -startdate 700101010101Z -days 7305 -extensions v3_ca -config openssl.cnf -out ca-cert.pem
cd -
