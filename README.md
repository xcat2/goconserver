## consoleserver (development stage)
This is a console server written in golang which can be integrate with [xcat3](https://github.com/chenglch/xcat3).
As openbmc is using ssh as the console, ssh plugin is implemented at first, ipmi or command plugin will be added in
the future.

## Setup

Setup golang and the environment of GOPATH and GOBIN at first. Then running the following command:

```
go get -v github.com/chenglch/consoleserver

```

## Rest API
Currently only rest api can be used to register or unregister the node.

```
[root@xcat3 ~]# curl -XPOST 'http://localhost:8089/nodes' -H Content-Type:application/json  -d '{"name": "node4", "driver": "ssh", "params": {"user":"chenglch", "host":"xx.xx.xx.xx","port":"22", "password":"xxxxx"}}'
[root@xcat3 ~]# curl -XGET 'http://localhost:8089/nodes' -H Content-Type:application/json
[root@xcat3 ~]# curl -XGET 'http://localhost:8089/nodes/node4' -H Content-Type:application/json
[root@xcat3 ~]# curl -XDELETE 'http://localhost:8089/nodes/node4' -H Content-Type:application/json
```

## TODO