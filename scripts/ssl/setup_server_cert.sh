#!/bin/bash

if [ -z "$CONSOLESERVER_DIR" ]; then
  CONSOLESERVER_DIR=/etc/consoleserver
fi
if [ -z "$1" ]; then
  echo "Usage: $0 servername"
  CNA=`hostname`
else
  CNA=$*
fi
umask 0077

CA_DIR=$CONSOLESERVER_DIR/ca

if [ -e $CONSOLESERVER_DIR/cert ]; then
  echo -n "$CONSOLESERVER_DIR/cert already exists, delete and start over (y/n)?"
  read ANSWER
  if [ "$ANSWER" != "y" ]; then
    echo "Aborting at user request"
    exit 0
  fi
  rm -rf $CONSOLESERVER_DIR/cert
fi
mkdir -p $CONSOLESERVER_DIR/cert
cd $CONSOLESERVER_DIR/cert
sed -i "s/#CONSOLESERVERCASAN#/DNS.1 = `hostname --long`\nDNS.2 = `hostname --short`/g" $CA_DIR/openssl.cnf
openssl genrsa -out server-key.pem 2048
openssl req -config $CA_DIR/openssl.cnf -new -key server-key.pem -out server-req.pem -extensions server -subj "/CN=$CNA"
cp server-req.pem  $CA_DIR/`hostname`.csr
cd -
cd $CA_DIR

openssl ca -startdate 600101010101Z -config openssl.cnf -in `hostname`.csr -out `hostname`.cert -extensions server -extensions san_env
if [ -f `hostname`.cert ]; then
    rm `hostname`.csr
fi

cp `hostname`.cert $CONSOLESERVER_DIR/cert/server-cert.pem
#Put key and cert in a single file for the likes of conserver
cat $CONSOLESERVER_DIR/cert/server-cert.pem $CONSOLESERVER_DIR/cert/server-key.pem > $CONSOLESERVER_DIR/cert/server-cred.pem
cp ca-cert.pem $CONSOLESERVER_DIR/cert/ca.pem
cd -