## consoleserver

`consoleserver` is written in golang and is a part of microservice of
[xcat3](https://github.com/chenglch/xcat3). It can work as a independent
tool to provide the terminal session service. Terminal session could run in
the background and help logging the terminal content.

### Support plugin
`consoleserver` intent to support multiple types of terminal plugins, currently
`ssh` is support as OpenBMC use `ssh` as the SOL method and `cmd` is a normal
driver to support command session.

### Structure
`consoleserver` can be divided into two parts:
- daemon part: `consoleserver`, expose rest api interface to define and control
  the session node.

- client part: `congo`, a command line tool to define session or connect to the
  session. Multiple client session could be shared.

## Setup

### Requirement

Currently, this tool is in the development stage, there is no binary release.
Please setup golang SDK and GOPATH environment at first.

### Build and installation

```
git clone https://github.com/chenglch/consoleserver.git
cd consoleserver
make install
```

### Setup SSL (optional)

Please refer to [ssl](/scripts/ssl/)

## Command Example

### Start service
daemon is running in the background.
```
consoleserver &
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
or command driver
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

Please refer to [rest api](/api/)
