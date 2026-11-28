// What one run of scripts/simulate_user_app.py --route entrance binds in the place of the app's
// customerEntranceDeployment.ts: the customer entrance of the run's store server and the order contract of its chain.
// The module is written into the run's own directory and named to the run's bundler alone (metro.config.js); nothing
// of it is kept in the repository, whose file binds nothing.
import {mkdirSync, writeFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {customerOrderBuild} from '../../../../p02-user-app/build-tools/customer-order-build';

export type RunBinding = {
  entrance: {origin: string; ca: string; pin: string};
  payment: {contract: string; asset: string; token?: object; rpc?: string};
};

/**
 * Writes the module and answers what it binds. The values pass the rules of a release's build file
 * (customer-order-build.js) first: what such a build could not bind is not bound here either, and stops the run.
 */
export function writeOrderBinding(file: string, run: RunBinding) {
  const {contract, asset, token, rpc} = run.payment;
  const build = {entrance: run.entrance, payment: {chain: 8283, contract, asset, ...(token ? {token} : {}), ...(rpc ? {rpc} : {})}};
  mkdirSync(dirname(file), {recursive: true});
  const record = join(dirname(file), 'customer-order-build.json');
  writeFileSync(record, JSON.stringify(build));
  const bound = customerOrderBuild(record) as {entrance: object; payment: object};
  writeFileSync(file, '// Written by one run of scripts/simulate_user_app.py for its own bundler; no build of the app reads it.\n' +
    `export const approvedCustomerEntrance = ${JSON.stringify(bound.entrance)};\nexport const approvedOrderPayment = ${JSON.stringify(bound.payment)};\n`);
  return bound;
}
