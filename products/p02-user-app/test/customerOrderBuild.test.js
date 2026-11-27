const {transformFileSync} = require('@babel/core');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const {customerOrderBuild} = require('../build-tools/customer-order-build');
const {entranceAnchor} = require('../src/merchant/customerEntrance');
const {orderPaymentPin} = require('../src/merchant/orderPaymentTrust');

const target = path.resolve(__dirname, '../src/merchant/customerEntranceDeployment.ts');
const entrance = {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};
const payment = {chain: 8283, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40),
  token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}, rpc: 'https://rpc.example'};
let folder;
beforeAll(() => { folder = fs.mkdtempSync(path.join(os.tmpdir(), 'nu54-order-build-')); });
afterAll(() => { fs.rmSync(folder, {recursive: true, force: true}); });

function written(build) {
  const file = path.join(folder, `build-${Math.random().toString(16).slice(2)}.json`);
  fs.writeFileSync(file, typeof build === 'string' ? build : JSON.stringify(build));
  return file;
}

function compile(build) {
  const prior = process.env.NU54_CUSTOMER_ORDER_BUILD;
  try {
    if (build === undefined) delete process.env.NU54_CUSTOMER_ORDER_BUILD;
    else process.env.NU54_CUSTOMER_ORDER_BUILD = written(build);
    return transformFileSync(target, {configFile: path.resolve(__dirname, '../babel.config.js')}).code;
  } finally {
    if (prior === undefined) delete process.env.NU54_CUSTOMER_ORDER_BUILD;
    else process.env.NU54_CUSTOMER_ORDER_BUILD = prior;
  }
}

test('the repository binds nothing: no entrance and no order contract', () => {
  const code = compile(undefined);
  expect(code.replace(/\s/g, '')).toContain('approvedCustomerEntrance=exports.approvedCustomerEntrance=undefined;');
  expect(code.replace(/\s/g, '')).toContain('approvedOrderPayment=exports.approvedOrderPayment=undefined;');
});

test('a build file binds the entrance anchor and the order contract at bundle time', () => {
  const code = compile({entrance, payment});
  for (const value of [entrance.origin, entrance.ca, entrance.pin, payment.contract, payment.asset, 'NU54 Test KRW', 'https://rpc.example', '8283'])
    expect(code).toContain(value);
  const listOnly = compile({entrance});
  expect(listOnly).toContain(entrance.pin);
  expect(listOnly).not.toContain(payment.contract);
});

test('a value a build may not bind stops the bundle', () => {
  for (const bad of ['{', {payment}, {entrance: {...entrance, origin: 'http://store.example'}}, {entrance: {...entrance, pin: '0'.repeat(64)}},
    {entrance, payment: {...payment, chain: 1}}, {entrance, payment: {...payment, contract: payment.asset}}, {entrance, payment: {...payment, contract: '0x' + 'A'.repeat(40)}},
    {entrance, payment: {...payment, rpc: 'http://rpc.example'}}, {entrance, payment: {...payment, signer: '0x' + '5'.repeat(40)}},
    {entrance, payment: {...payment, merchantId: 'e'.repeat(64)}}, {entrance, payment, server: 'https://x.example'}])
    expect(() => compile(bad)).toThrow('NU54_CUSTOMER_ORDER_BUILD');
});

test('a build may bind the current server key and the next one', () => {
  const next = 'cd'.repeat(32);
  const code = compile({entrance: {...entrance, pin: [entrance.pin, next]}});
  expect(code).toContain(entrance.pin);
  expect(code).toContain(next);
  for (const pin of [[], [entrance.pin, entrance.pin], [entrance.pin, next, 'ef'.repeat(32)], [entrance.pin, '0'.repeat(64)], [entrance.pin, 'AB'.repeat(32)]])
    expect(() => compile({entrance: {...entrance, pin}})).toThrow('NU54_CUSTOMER_ORDER_BUILD');
});

// The app takes a value it refuses as no value, which would ship a build that silently lists no store or takes no
// order. So whatever the app refuses must stop the bundle, and whatever the bundle takes the app must take.
const coin = '0x' + '1000'.padStart(40, '0');
const tokens = [
  undefined, payment.token, {...payment.token, decimals: 8}, {...payment.token, decimals: 18, boardDecimals: 6},
  {...payment.token, decimals: 0}, {...payment.token, decimals: 19}, {...payment.token, decimals: 2.5}, {...payment.token, decimals: '2'},
  {...payment.token, decimals: 9}, {...payment.token, decimals: 18, boardDecimals: 9}, {...payment.token, decimals: 18, boardDecimals: 1},
  {...payment.token, boardDecimals: 2}, {...payment.token, name: ''}, {...payment.token, name: ' NU54'}, {...payment.token, name: 'x'.repeat(65)},
  {...payment.token, name: '원'}, {...payment.token, symbol: ''}, {...payment.token, symbol: '1KRW'}, {...payment.token, symbol: 't-KRW'},
  {...payment.token, symbol: 'x'.repeat(17)}, {...payment.token, label: 'tKRW 원'}, {...payment.token, label: ''},
  {name: payment.token.name, symbol: 'tKRW', decimals: 2}, {...payment.token, note: 'x'}, {...payment.token, symbol: 'WKRC', label: 'KRWC'},
  {...payment.token, symbol: 'WKRC'}, {...payment.token, label: 'KRWC'}, null, 'tKRW', [],
];
test.each(tokens.flatMap(token => [payment.asset, coin].map(asset => [JSON.stringify(token) ?? 'no token', asset, token])))(
  'the bundle takes the token %s for the asset %s exactly when the app does', (_name, asset, token) => {
    const pin = {chain: 8283, contract: payment.contract, asset, ...(token === undefined ? {} : {token})};
    let bound;
    try { bound = customerOrderBuild(written({entrance, payment: pin})).payment; } catch (error) { expect(error.message).toContain('NU54_CUSTOMER_ORDER_BUILD: payment'); }
    expect(bound !== undefined).toBe(orderPaymentPin(pin) !== undefined);
  });

test.each(['https://store.example:8444', 'https://store.example', 'https://[::1]:8444', 'https://store.example:0', 'https://store.example:65536',
  'https://-store.example', 'https://store..example', 'https://store.example/', 'https://store.example/v2', 'https://user@store.example',
  'http://store.example', 'https://store.example:8444?x=1', ''])('the bundle takes the origin %s only when the app does', origin => {
  let bound;
  try { bound = customerOrderBuild(written({entrance: {...entrance, origin}})).entrance; } catch (error) { expect(error.message).toContain('NU54_CUSTOMER_ORDER_BUILD: entrance'); }
  // The bundle is the stricter one about a trailing slash; it never takes what the app refuses.
  if (bound) expect(entranceAnchor(bound)).toBeDefined();
  else if (origin !== 'https://store.example/') expect(entranceAnchor({...entrance, origin})).toBeUndefined();
});
