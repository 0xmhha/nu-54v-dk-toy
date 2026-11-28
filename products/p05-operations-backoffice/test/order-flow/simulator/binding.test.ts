// Where the simulation's binding of the customer entrance may be, and where it may not. A run of
// scripts/simulate_user_app.py --route entrance binds the entrance and the order contract of that run in this
// directory's bundler configuration alone. The repository's file, the app's tests and the app's own bundler bind
// nothing, with or without a simulation beside them.
import {transformFileSync} from '@babel/core';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, readdirSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {configuredEntrance, entranceAnchor} from '../../../../p02-user-app/src/merchant/customerEntrance';
import {approvedCustomerEntrance, approvedOrderPayment} from '../../../../p02-user-app/src/merchant/customerEntranceDeployment';
import {orderPaymentPin} from '../../../../p02-user-app/src/merchant/orderPaymentTrust';
import {ordersWithoutRegistration} from '../../../../p02-user-app/src/merchant/orderRoute';
import {configuredDirectory} from '../../../../p02-user-app/src/merchant/storeDirectory';
import {writeOrderBinding} from './orderBinding';

const app = resolve(__dirname, '../../../../p02-user-app');
const deployment = join(app, 'src/merchant/customerEntranceDeployment.ts');
const run = {
  entrance: {origin: 'https://127.0.0.1:56239', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)},
  payment: {contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40), token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}, rpc: 'https://rpc.test'},
};
let folder: string;
beforeAll(() => { folder = mkdtempSync(join(tmpdir(), 'nu54-order-binding-')); });
afterAll(() => { rmSync(folder, {recursive: true, force: true}); });

/**
 * What a bundler configuration resolves requests to, asked in a process of its own so that each asking reads the
 * environment anew. A request the configuration leaves alone comes back as 'the app's own file'.
 */
function resolved(config: string, binding: string | undefined, requests: string[]) {
  const script = `
    const config = require(${JSON.stringify(config)});
    const context = {originModulePath: ${JSON.stringify(join(app, 'src/merchant/storeAccess.ts'))}, resolveRequest: () => "the app's own file"};
    const answers = ${JSON.stringify(requests)}.map(name => {
      const found = config.resolver.resolveRequest(context, name, 'android');
      return typeof found === 'string' ? found : found.filePath;
    });
    process.stdout.write(JSON.stringify({answers, watched: config.watchFolders}));`;
  const env = {...process.env};
  delete env.NU54_SIMULATOR_ORDER_BINDING;
  delete env.NU54_CUSTOMER_ORDER_BUILD;
  if (binding) env.NU54_SIMULATOR_ORDER_BINDING = binding;
  return JSON.parse(execFileSync(process.execPath, ['-e', script], {cwd: app, env, encoding: 'utf8'})) as {answers: string[]; watched: string[]};
}
const requests = ['./customerEntranceDeployment.ts', '../merchant/customerEntranceDeployment.ts', './customerEntrance.ts', '../specs/NativeCommerceHttp.ts'];

test('the repository binds no entrance and no order contract', () => {
  const source = readFileSync(deployment, 'utf8');
  expect(source).toMatch(/^export const approvedCustomerEntrance: CustomerEntranceAnchor \| undefined = undefined;$/m);
  expect(source).toMatch(/^export const approvedOrderPayment: OrderPaymentPin \| undefined = undefined;$/m);
});

test('the app under Jest sees neither, and takes the route with a store registration', () => {
  expect(approvedCustomerEntrance).toBeUndefined();
  expect(approvedOrderPayment).toBeUndefined();
  expect(ordersWithoutRegistration()).toBe(false);
  expect(configuredEntrance()).toBeUndefined();
  expect(configuredDirectory()).toBeUndefined();
});

test('a bundle of the repository leaves both undefined, also while a simulation names its binding', () => {
  const file = join(folder, 'named', 'customerEntranceDeployment.js');
  writeOrderBinding(file, run);
  const prior = {...process.env};
  try {
    delete process.env.NU54_CUSTOMER_ORDER_BUILD;
    process.env.NU54_SIMULATOR_ORDER_BINDING = file;
    const code = transformFileSync(deployment, {configFile: join(app, 'babel.config.js')})!.code!.replace(/\s/g, '');
    expect(code).toContain('approvedCustomerEntrance=exports.approvedCustomerEntrance=undefined;');
    expect(code).toContain('approvedOrderPayment=exports.approvedOrderPayment=undefined;');
    expect(code).not.toContain(run.entrance.pin);
  } finally {
    for (const name of ['NU54_CUSTOMER_ORDER_BUILD', 'NU54_SIMULATOR_ORDER_BINDING'])
      if (prior[name] === undefined) delete process.env[name]; else process.env[name] = prior[name];
  }
});

test("the app's own bundler never takes the simulation's binding", () => {
  const file = join(folder, 'release', 'customerEntranceDeployment.js');
  writeOrderBinding(file, run);
  for (const binding of [undefined, file]) {
    const {answers, watched} = resolved(join(app, 'metro.config.js'), binding, requests);
    expect(answers).toEqual(requests.map(() => "the app's own file"));
    expect(watched.some(place => file.startsWith(place + '/'))).toBe(false);
  }
});

test("the simulation's bundler takes the binding only when a run names it, and only for that one module", () => {
  const file = join(folder, 'run', 'customerEntranceDeployment.js');
  writeOrderBinding(file, run);
  const config = join(__dirname, 'metro.config.js');
  const plain = resolved(config, undefined, requests);
  expect(plain.answers).toEqual(["the app's own file", "the app's own file", "the app's own file", join(__dirname, 'NativeCommerceHttp.ts')]);
  const bound = resolved(config, file, requests);
  expect(bound.answers).toEqual([file, file, "the app's own file", join(__dirname, 'NativeCommerceHttp.ts')]);
  expect(bound.watched).toEqual([...plain.watched, join(folder, 'run')]);
});

test('what a run binds is what the app takes as an entrance and an order contract', () => {
  const file = join(folder, 'taken', 'customerEntranceDeployment.js');
  const bound = writeOrderBinding(file, run) as {entrance: typeof run.entrance; payment: {chain: 8283; contract: string; asset: string}};
  expect(entranceAnchor(bound.entrance)).toBeDefined();
  expect(orderPaymentPin(bound.payment)).toMatchObject({chain: 8283, contract: run.payment.contract, asset: run.payment.asset, token: run.payment.token});
  expect(ordersWithoutRegistration(bound.entrance, bound.payment)).toBe(true);
  const written = readFileSync(file, 'utf8');
  expect(written).toContain(`export const approvedCustomerEntrance = ${JSON.stringify(bound.entrance)};`);
  expect(written).toContain(`export const approvedOrderPayment = ${JSON.stringify(bound.payment)};`);
  // The dollar store names no token.
  const dollars = writeOrderBinding(join(folder, 'dollars', 'customerEntranceDeployment.js'), {...run, payment: {contract: run.payment.contract, asset: run.payment.asset}});
  expect(orderPaymentPin(dollars.payment as {chain: 8283; contract: string; asset: string})).toEqual({chain: 8283, contract: run.payment.contract, asset: run.payment.asset});
});

test('a value a release could not bind stops the run instead of being bound', () => {
  for (const bad of [{...run, entrance: {...run.entrance, origin: 'http://127.0.0.1:56239'}}, {...run, entrance: {...run.entrance, pin: '0'.repeat(64)}},
    {...run, payment: {...run.payment, contract: run.payment.asset}}, {...run, payment: {...run.payment, rpc: 'http://rpc.test'}}])
    expect(() => writeOrderBinding(join(folder, 'bad', 'customerEntranceDeployment.js'), bad)).toThrow('NU54_CUSTOMER_ORDER_BUILD');
});

test('no binding is kept beside the stand-ins', () => {
  expect(readdirSync(__dirname).filter(name => /customerEntranceDeployment|customer-order-build/.test(name))).toEqual([]);
});
