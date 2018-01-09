## goconserver

`goconserver` is written in golang and is a part of microservice of
[xcat3](https://github.com/chenglch/xcat3). It can work as a independent
tool to provide the terminal session service. Terminal session could run in
the background and help logging the terminal content.

### Support plugin
`goconserver` intent to support multiple types of terminal plugins, currently
`ssh` is support as OpenBMC use `ssh` as the SOL method and `cmd` is a general
driver to support command session.

### Structure
`goconserver` can be divided into two parts:
- daemon part: `goconserver`, expose rest api interface to define and control
  the session node.

- client part: `congo`, a command line tool to define session or connect to the
  session. Multiple client sessions could be shared.

## Setup goconserver from release

Download the tarball for release from
[goconserver](https://github.com/chenglch/goconserver/releases), take the
release for amd64 architecture as a example.
```
wget https://github.com/chenglch/goconserver/files/1437496/goconserver_linux_amd64.tar.gz
tar xvfz goconserver_linux_amd64.tar.gz
cd goconserver_linux_amd64
./setup.sh
```

Modify the congiguration file `/etc/goconserver/server.conf` based on your
environment, then run `goconserver` to start the daemon service. To support a
large amount of sessions, please use `ulimit -n <number>` command to set the
number of open files.
```
goconserver [--congi-file <file>]
```

Modify the the environment variables in `/etc/profile.d/congo.sh` based on your
environment, then try the `congo` command.
```
source /etc/profile.d/congo.sh
congo list
```

## Development

### Requirement

Please setup golang SDK, GOPATH environment variable and glide tool at first.

### Build and install

```
git clone https://github.com/chenglch/goconserver.git
cd goconserver
make deps
make install
```

### Setup SSL (optional)

Please refer to [ssl](/scripts/ssl/)

## Command Example

### Start service
```
goconserver &
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
