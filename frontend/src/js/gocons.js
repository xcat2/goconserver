const STATUS_CONNECTED = 2;

const ACTION_SESSION_ERROR = 0
const ACTION_SESSION_START = 1
const ACTION_SESSION_DROP = 2
const ACTION_SESSION_REDIRECT = 3
const ACTION_SESSION_OK = 4
const ACTION_SESSION_RETRY = 5

const Terminal = require('xterm').Terminal;
Terminal.applyAddon(require('xterm/lib/addons/fit'));
const host = window.location.host;
const utils = require("./utils.js");

class TLVBuf {
    constructor() {
        this.buf = new Buffer("");
        this.n = 0;
    }
}

class ConsoleSession {
    constructor(node, termBox, windowDiv, usersDiv) {
        if (!$.trim(node) || !termBox) {
            throw new Error("Parameter node or termBox could not be null");
        }
        this.node = $.trim(node);
        this.termBox = termBox;
        this.windowDiv = windowDiv;
        this.usersDiv = usersDiv;
        this.url = (window.location.protocol === "https:" ? 'wss://' : 'ws://') + window.location.host + window.location.pathname + "session";
        this.ws = new WebSocket(this.url, ['tty']);
        this.action = ACTION_SESSION_START;
        this.term = this.openTerm();
        this.tlv = new TLVBuf();
        if (this.windowDiv) {
            $("#" + windowDiv).html("Console Window For " + node);
        }
        this.initWs();
        this.term.on('data', (data) => {
            if (this.action != ACTION_SESSION_OK) {
                return;
            }
            if (this.ws.readyState == 3) {
                this.disable();
                return;
            }
            if (data) {
                this.ws.send(utils.int32toBytes(data.length) + data);
            }
        });
    }
    initWs() {
        this.ws = new WebSocket(this.url, ['tty']);
        this.ws.onopen = function(event) {
            if (this.ws.readyState === WebSocket.OPEN) {
                let msg = JSON.stringify({
                    node: this.node,
                    action: ACTION_SESSION_START,
                });
                this.ws.send(utils.int32toBytes(msg.length) + msg);
                this.getUser();
                this.timer = setInterval(this.getUser, 5000, this);
            }
        }.bind(this);

        this.ws.onclose = function(event) {
            if (this.action == ACTION_SESSION_REDIRECT) {
                if (this.timer) {
                    clearInterval(this.timer);
                }
                this.initWs();
            } else {
                console.log('Websocket connection closed with code: ' + event.code);
                this.disable();
            }
        }.bind(this);

        this.ws.onmessage = (event) => {
            if (!event.data) {
                return;
            }
            if (this.ws.readyState == 3) {
                this.disable();
                return;
            }
            // a work around to convert the data info base64 format to avoid of utf-8 error from websocket
            let data = Buffer.from(event.data, 'base64');
            let msg = this.getMessage(data);
            if (this.action != ACTION_SESSION_OK && this.action != ACTION_SESSION_ERROR) {
                let obj = JSON.parse(msg);
                switch (obj.action) {
                    case ACTION_SESSION_DROP:
                        this.action = ACTION_SESSION_ERROR;
                        console.log("Failed to start console, status=" + this.state)
                        break;
                    case ACTION_SESSION_OK:
                        this.action = ACTION_SESSION_OK
                        break;
                    case ACTION_SESSION_REDIRECT:
                        this.url = (window.location.protocol === "https:" ? 'wss://' : 'ws://') + obj.host + ":" + obj.api_port + window.location.pathname + "session";
                        this.ws.close();
                        this.action = ACTION_SESSION_REDIRECT
                        break;
                }
            } else if (this.action == ACTION_SESSION_OK) {
                if (msg) {
                    this.term.write(msg);
                }
            }
        };

    }
    openTerm() {
        let terminalContainer = document.getElementById(this.termBox);
        let term = new Terminal({
            cursorBlink: true
        });
        term.open(terminalContainer);
        term.fit();
        window.addEventListener('resize', function() {
            clearTimeout(window.resizedFinished);
            window.resizedFinished = setTimeout(function() {
                term.fit();
            }, 250);
        });
        return term;
    }
    getUser(s) {
        if (!s) {
            return;
        }
        if (s.usersDiv) {
            new Users(s.node, s.usersDiv);
        }
    }
    getMessage(data) {
        let msg = "";
        this.tlv.buf = Buffer.concat([this.tlv.buf, data]);
        while (1) {
            if (this.tlv.n == 0) {
                if (this.tlv.buf.length < 4) {
                    break;
                }
                this.tlv.n = utils.bytesToInt32(this.tlv.buf);
                this.tlv.buf = this.tlv.buf.slice(4, this.tlv.buf.length);
            }
            if (this.tlv.buf.length < this.tlv.n) {
                break;
            }
            msg += this.tlv.buf.slice(0, this.tlv.n);
            this.tlv.buf = this.tlv.buf.slice(this.tlv.n, this.tlv.buf.length);
            this.tlv.n = 0;
        }
        return msg;
    }

    disable() {
        this.ws.close();
        this.term.write("\r\nSession has been closed.\r\n");
        this.term.setOption('disableStdin', true);
        if (this.windowDiv) {
            let node = $.urlParam("node");
            if (this.node != node) {
                return;
            }
            $("#" + this.windowDiv).html("Console Window For " + this.node + " (closed)");
        }
    }

    close() {
        if (this.timer) {
            clearInterval(this.timer);
        }
        this.ws.close();
        this.term.destroy();
        if (this.windowDiv) {
            $("#" + this.windowDiv).html("&nbsp;");
        }
    }
}

class Users {
    constructor(node, usersDiv) {
        if (!$.trim(node) || !usersDiv) {
            throw new Error("Parameter node or usersDiv could not be null");
        }
        this.header = $("#" + usersDiv).html();
        this.node = node;
        this.usersDiv = usersDiv
        this.initUsers();
    }
    initUsers() {
        $.ajax({
            url: "command/user/" + this.node,
            type: "GET",
            dataType: "json"
        }).then(function(data) {
            this.users = data["users"];
            this.renderUsers();
        }.bind(this), function() {
            console.log("Faled to get response form /command/user/.")
        })
    }
    renderUsers() {
        let text = "";
        this.users.sort();
        for (let i in this.users) {
            let line = '<li>' + this.users[i] + '</li>';
            text += line;
        }
        $("#" + this.usersDiv).html(text);
    }
}

class Nodes {
    constructor(nodeDiv) {
        if (!nodeDiv) {
            throw new Error("Parameter nodeDiv could not be null.")
        }
        this.header = '<tr><th>NODE</th><th>HOST</th><th>STATE</th></tr>';
        this.nodeDiv = nodeDiv;
        this.initNodes();
    }
    initNodes() {
        $.ajax({
            url: "nodes",
            type: "GET",
            dataType: "json"
        }).then(function(data) {
            console.log(data);
            this.nodes = data["nodes"];
            this.sortNodes();
            this.renderNodes();

        }.bind(this), function() {
            console.log("Faled to get response form /nodes.")
        })
    }

    sortNodes() {
        this.nodes.sort(function(a, b) {
            if (a["name"] > b["name"]) {
                return 1;
            } else if (a["name"] == b["name"]) {
                return 0;
            }
            return -1;
        });
    }

    renderNodes() {
        let text = this.header;
        for (let i in this.nodes) {
            let node = this.nodes[i];
            let line = '<tr><td><a href="#!/?node=' + node["name"] + '">' + node["name"] + '</a></td><td>' +
                node["host"] + '</td><td>' +
                node["state"] + '</td></tr>';
            text += line;
        }
        $("#" + this.nodeDiv).html(text);
    }
}

exports.ConsoleSession = ConsoleSession;
exports.Users = Users;
exports.Nodes = Nodes;