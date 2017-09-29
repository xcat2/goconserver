## Rest API example

### Create node session
POST /nodes
```
{
    "name": "node2",
    "driver": "ssh",
    "params": {
        "user": "chenglch",
        "host": "10.0.0.2",
        "port": "22",
        "private_key": "~/.ssh/id_rsa"
    }
}
```

### Bulk create
POST /bulk/nodes
```
{
    "nodes": [
        {
            "name": "bulknode1",
            "ondemand": false,
            "driver": "ssh",
            "params": {
                "user": "root",
                "host": "10.0.0.1",
                "port": "22",
                "password": "password"
            }
        },
        {
            "name": "bulknode2",
            "driver": "ssh",
            "ondemand": true,
            "params": {
                "user": "root",
                "host": "10.0.0.2",
                "port": "22",
                "password": "password"
            }
        }
    ]
}
```
### List node session
GET /nodes

### Show the detail of node session
GET /nodes/<node>

### Control the node session
PUT /nodes/<node>?state=on    # console session will connect in the background

PUT /nodes/<node>?state=off   # console session will disconnect in the background

### Bulk Control
PUT /bulk/nodes?state=on
```
{
    "nodes": [
        {
            "name": "bulknode1"
        },
        {
            "name": "bulknode2"
        }
    ]
}
```

### Delete node session
DELETE /nodes/<node>

### Bulk delete
DELETE /bulk/nodes
```
{
    "nodes": [
        {
            "name": "bulknode1"
        },
        {
            "name": "bulknode2"
        }
    ]
}
```
