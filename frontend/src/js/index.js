import img from '../img/console.png';
const utils = require("./utils.js");
const gocons = require("./gocons.js");
let session;

function loadNodes() {
    try {
        new gocons.Nodes("nodes");
    } catch (e) {
        console.log("Failed to load nodes data, error: " + e.message);
    }

}

function hashChange() {
    if (session) {
        session.close();
        session = null;
    }
    loadNodes();
    let node = $.urlParam("node");
    if ($.trim(node)) {
        try {
            session = new gocons.ConsoleSession(node, "term-box", "console_window", "users");
        } catch (e) {
            console.log("Could not create session object, error: " + e.message);
        }
    }
}

$(function() {
    loadNodes();
    setInterval(loadNodes, 5000);
    let node = $.urlParam("node");
    $("#middle").height($("#middle").width());
    window.onhashchange = hashChange;
    if ($.trim(node)) {
        try {
            session = new gocons.ConsoleSession(node, "term-box", "console_window", "users");
        } catch (e) {
            console.log("Could not create session object, error: " + e.message);
        }
    }
});