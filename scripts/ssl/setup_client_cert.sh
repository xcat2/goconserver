#!/bin/bash

if [ -z "$CONSOLESERVER_DIR" ]; then
  CONSOLESERVER_DIR=/etc/consoleserver
fi
# if `-f`|`--force` option is supplied, set a variable and remove option from paraameters
# This allows use of the old code without modification
FORCE=0
for v in "$@"; do
  case "$v" in
    "-f"|"--force")
      FORCE=1
      continue
      ;;
  esac
  ARGS[${#ARGS[@]}]="$v"
done
if [ ${#ARGS[@]} -gt 0 ]; then
  set "${ARGS[@]}"
fi
if [ -z "$1" ]; then
  set `whoami`
fi
# if directory is not supplied then just use home
if [ -z "$2" ]; then
  CNA="$*"
# getent doesn't exist on AIX
  if [ -x /usr/bin/getent ];then
    USERHOME=`getent passwd $1|awk -F: '{print $6}'`
  else
    USERHOME=`grep ^$1: /etc/passwd | cut -d: -f6`
  fi
else
  CNA="$1"
  USERHOME=$2
fi
CA_DIR=$CONSOLESERVER_DIR/ca

if [ -e $USERHOME/.congo ]; then
# exit 0
  if [ $FORCE -eq 0 ]; then
    echo -n "$USERHOME/.congo already exists, delete and start over (y/n)?"
    read ANSWER
    if [ "$ANSWER" != "y" ]; then
      echo "Aborting at user request"
      exit 0
    fi
  fi
  rm -rf $USERHOME/.congo
fi
# remove user from index
index=`grep $CNA $CA_DIR/index | cut -f4 2>&1`
for id in $index; do
  openssl ca -startdate 19600101010101Z -config $CA_DIR/openssl.cnf -revoke $CA_DIR/certs/$id.pem
done
mkdir -p $USERHOME/.congo
cd $USERHOME/.congo
openssl genrsa -out client-key.pem 2048
if [ $FORCE -eq 0 ]; then
  openssl req -config $CA_DIR/openssl.cnf -new -key client-key.pem -out client-req.pem -extensions usr_cert -subj "/CN=$CNA"
else
  openssl req -config $CA_DIR/openssl.cnf -new -key client-key.pem -out client-req.pem -extensions usr_cert -subj "/CN=$CNA" -batch
fi
cp client-req.pem $CA_DIR/root.csr
cd - >/dev/null
cd $CA_DIR

#   - "make sign" doesn't work on my AIX test system????
#   - seems to be a problem with the use of the wildcard in the Makefile
#   - calling cmds directly instead - should be safe
# make sign
if [ $FORCE -eq 0 ]; then
  openssl ca -startdate 600101010101Z -config openssl.cnf -in root.csr -out root.cert
else
  openssl ca -startdate 600101010101Z -config openssl.cnf -in root.csr -out root.cert -batch
fi
if [ -f root.cert ]; then
  rm root.csr
fi

cp root.cert $USERHOME/.congo/client-cert.pem
#Unify certificate and key in one file, console command at least expects it
cat $USERHOME/.congo/client-cert.pem $USERHOME/.congo/client-key.pem > $USERHOME/.congo/client-cred.pem
cp ca-cert.pem $USERHOME/.congo/ca.pem
chown -R $1 $USERHOME/.congo
find $USERHOME/.congo -type f -exec chmod 600 {} \;
find $USERHOME/.congo -type d -exec chmod 700 {} \;
chmod 644 $USERHOME/.congo/ca.pem
chmod 755 $USERHOME/.congo
cd - >/dev/null
