## Generate openssl certs
These code is borrowed from xcat-core (https://github.com/xcat2/xcat-core)

```
./setup_ca_cert.sh "CONGO"
./setup_server_cert.sh
./setup_client_cert.sh
```

## Configure the SSL certification
cat /etc/consoloserver/server.conf
```
global:
  host: 0.0.0.0
  ssl_key_file: /etc/goconserver/cert/server-key.pem
  ssl_cert_file: /etc/goconserver/cert/server-cert.pem
  ssl_ca_cert_file: /etc/goconserver/cert/ca.pem
```

cat /etc/profile.d/congo.sh
```
export CONGO_URL="https://node910f05c01bc02k74:8089"
export CONGO_SERVER_HOST="node910f05c01bc02k74"
export CONGO_PORT="12430"
export CONGO_CONSOLE_TIMEOUT=3
export CONGO_SSL_KEY=/root/.congo/client-key.pem
export CONGO_SSL_CERT=/root/.congo/client-cred.pem
export CONGO_SSL_CA_CERT=/root/.congo/ca.pem
```

source /etc/profile.d/congo.sh