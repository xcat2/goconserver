## goconserver

`goconserver` is written in golang which intend to replace the `conserver`
which used in [xcat2](https://github.com/xcat2/xcat-core). The microservice
based design makes it easy to integrate with other tool which hope to log the
terminal sessions. It can also work independently through command line or rest
api interface.

![preview](/goconserver.gif)

## Key Features

- Manage the lifecycle of seesion hosts via REST or BULK REST interface.
- Interface based desgin, support multiple types of terminal, storage and
  output plugin. Multiple output plugins could work together.
- Multiple client could share one host session.

### Terminal plugins

- ssh: SSH driver start ssh session within goruntine, As no external process,
       goconserver could support a large number of OpenBMC
       [openbmc](https://github.com/openbmc) consoles with high performance.

- cmd: A general driver to help redirect the command input and output. Any
       shell based terminal type could be supported.

### Output plugins:

- file: Store the terminal session in files for different hosts.
- tcp:  Send the console line (splitted) in json format to the remote target
        with tcp method.
- udp:  Send the console line (splitted) in json format to the remote target
        with udp method.

### Storage plugins

- file: Store the host information in a json file.
- etcd: Support goconserver cluster.

### Multiple client types

- terminal: Get console session via TCP(or with TLS) connection.
- web: Get console session from web terminal.

![preview](/goconserver2.gif)

### Design Structure
`goconserver` can be divided into three parts:
- daemon part: `goconserver`, expose REST api interface to define and control
  the session host.

- client part: `congo`, a command line tool to manage the configuration of
  session hosts. A tty console client is also provided and multiple clients
  could share the same session.

- frontend part: A web page is provided to list the session status and expose
  a web terminal for the selected node. The tty client from `congo` can share
  the same session from web browser.

## Setup goconserver from release

### Setup
Download binary or RPM tarball from
[goconserver](https://github.com/xcat2/goconserver/releases)
```
yum install <goconserver.rpm>
systemctl start goconserver.service
```

### Configuration
For the server side, modify the congiguration file
`/etc/goconserver/server.conf` based on your environment, then restart
goconserver service.

For client, modify the the environment variables in `/etc/profile.d/congo.sh`
based on your environment, then try the `congo` command. For example:

```
source /etc/profile.d/congo.sh
congo list
```

### Start goconserver with xcat

Currently `xcat` and `goconserver` are integrated with `file` storage type.
Some reference doc could be found at

- [rcons](http://xcat-docs.readthedocs.io/en/latest/guides/admin-guides/manage_clusters/ppc64le/management/basic/rcons.html)
- [gocons](http://xcat-docs.readthedocs.io/en/latest/advanced/goconserver/index.html)

## Development

### Requirement

Please setup golang SDK(1.9 or higher), GOPATH environment variable and
[glide](https://github.com/Masterminds/glide) tool at first.

### Build and install

```
git clone https://github.com/xcat2/goconserver.git
cd goconserver
make deps
make install
```

### Setup SSL/TLS (optional)

Please refer to [ssl](/scripts/ssl/)

### Web Interface

Setup nodejs(9.0+) and npm(5.6.0+) toolkit at first. An example steps could be
found at [node env](/frontend/). Then follow the steps below:

```
yum install gcc-c++
npm install -g gulp webpack webpack-cli
make frontend
```

The frontend content is generated at `build/dist` directory. To enable it,
modify the configuration in `/etc/gconserver/server.conf` like below, then
restart `goconserver` service. The web url is available on
`http(s)://<ip or domain name>:<api-port>/`.
```
api:
  dist_dir : "<dist directory>"
```

## Command Example

### Start service
```
goconserver &                     # for debug
service goconserver start         # only support systemd system
```
### Define testnode node session
congo is the client command. Use congo help to see the detail.
```
congo create testnode driver=ssh ondemand=false --params user=root,host=10.5.102.73,port=22,password=<password>
```
or with ssh private key
```
congo create testnode driver=ssh ondemand=false --params user=root,host=10.5.102.73,port=22,private_key=<priavte_key_path>
```
or general command driver
```
congo create testnode driver=cmd ondemand=false --params cmd="ssh -l root -p 22 10.5.102.73"
```

### List or show detail
```
congo list
congo show testnode
```

### Connect to the testnode session
```
congo console testnode
```

## Rest API

Rest api support bulk interface to manage the console sessions.
Please refer to [rest api](/api/) for detail.
