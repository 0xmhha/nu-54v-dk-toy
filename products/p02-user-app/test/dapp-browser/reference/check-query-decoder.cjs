const assert = require('node:assert/strict');
require('./query-decoder-interop.cjs');
const {parse} = require('query-string');
assert.equal(parse('relay=wss%3A%2F%2F127.0.0.1%3A1234').relay, 'wss://127.0.0.1:1234');
assert.equal(parse('invalid=%F0%9F%92').invalid, '%F0%9F%92');
