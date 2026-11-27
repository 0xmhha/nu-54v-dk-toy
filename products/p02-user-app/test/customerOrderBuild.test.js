const {transformFileSync} = require('@babel/core');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const target = path.resolve(__dirname, '../src/merchant/customerEntranceDeployment.ts');
const entrance = {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};
const payment = {chain: 8283, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40),
  token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}, rpc: 'https://rpc.example'};
let folder;
beforeAll(() => { folder = fs.mkdtempSync(path.join(os.tmpdir(), 'nu54-order-build-')); });
afterAll(() => { fs.rmSync(folder, {recursive: true, force: true}); });

function compile(build) {
  const prior = process.env.NU54_CUSTOMER_ORDER_BUILD;
  try {
    if (build === undefined) delete process.env.NU54_CUSTOMER_ORDER_BUILD;
    else {
      const file = path.join(folder, `build-${Math.random().toString(16).slice(2)}.json`);
      fs.writeFileSync(file, typeof build === 'string' ? build : JSON.stringify(build));
      process.env.NU54_CUSTOMER_ORDER_BUILD = file;
    }
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
