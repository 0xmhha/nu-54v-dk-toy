// The official Apache SDK uses query-string 7's CommonJS decoder import.
// Node 22 loads the patched decoder's ESM namespace synchronously. Give that
// exact import its default function before loading SignClient.
const {createRequire} = require('node:module');
const fromQueryString = createRequire(require.resolve('query-string'));
const decoderPath = fromQueryString.resolve('decode-uri-component');
const decoder = fromQueryString(decoderPath);
if (typeof decoder.default !== 'function' || !require.cache[decoderPath]) {
  throw new Error('Pinned decoder module format changed');
}
require.cache[decoderPath].exports = decoder.default;
