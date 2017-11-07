#!/bin/bash
# Setup script to setup release binary

set -x

BASEPATH=$(cd `dirname $0`; pwd)

cp $BASEPATH/goconserver $BASEPATH/congo /usr/local/bin/
mkdir -p /etc/goconserver /var/log/goconserver/nodes /var/lib/goconserver
chmod 700 /etc/goconserver /var/log/goconserver/nodes /var/lib/goconserver

if [ ! -f "/etc/goconserver/server.conf" ];  then \
	cp $BASEPATH/etc/goconserver/server.conf /etc/goconserver/; \
fi;
if [ ! -f "/etc/profile.d/congo.sh" ]; then \
	cp $BASEPATH/etc/goconserver/client.sh /etc/profile.d/congo.sh; \
fi

if [  -d "/usr/lib/systemd/" ]; then
    mkdir -p /usr/lib/systemd/system
    cp $BASEPATH/etc/systemd/goconserver.service /usr/lib/systemd/system/
    systemctl daemon-reload
fi