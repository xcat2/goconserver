$.urlParam = function(name) {
    var results = new RegExp('[\?&]' + name + '=([^&#]*)').exec(window.location.href);
    if (results == null) {
        return null;
    } else {
        return decodeURI(results[1]) || 0;
    }
}

let int32toBytes = function(num) {
    let buf = new Buffer(4);
    buf.writeUInt32BE(num, 0);
    return buf;
};

let bytesToInt32 = function(numString) {
    let buf = Buffer.from(numString);
    return buf.readInt32BE(0);
};

exports.int32toBytes = int32toBytes;
exports.bytesToInt32 = bytesToInt32;