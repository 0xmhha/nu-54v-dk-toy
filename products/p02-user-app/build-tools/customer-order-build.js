const fs = require('node:fs');
const path = require('node:path');

// Binds what a phone trusts for ordering without a store registration (src/merchant/customerEntranceDeployment.ts):
// the customer entrance of the store server and the order contract. NU54_CUSTOMER_ORDER_BUILD names a JSON file
//   {"entrance": {"origin": "https://…", "ca": "<DER in base64>", "pin": "<64 hex>"},
//    "payment":  {"chain": 8283, "contract": "0x…", "asset": "0x…", "token": {…}, "rpc": "https://…"}}
// `payment` is optional (a build that lists stores but takes no order) and needs `entrance`. Unset, both stay
// undefined, which is what the repository ships. The app checks the same values again when it starts.

const address = value => typeof value === 'string' && /^0x[0-9a-f]{40}$/.test(value) && !/^0x0+$/.test(value);
const https = value => typeof value === 'string' && value.length <= 512 && /^https:\/\/[A-Za-z0-9.[\]:_-]+(?:\/[A-Za-z0-9/_.-]*)?$/.test(value);
const only = (value, names) => !!value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).every(name => names.includes(name));
const fail = part => { throw new Error(`NU54_CUSTOMER_ORDER_BUILD: ${part} is not a value a build may bind`); };

function customerOrderBuild(file) {
  if (!file) return {};
  let build;
  try {
    build = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch {
    fail('the file');
  }
  if (!only(build, ['entrance', 'payment'])) fail('the file');
  const {entrance, payment} = build;
  if (!only(entrance, ['origin', 'ca', 'pin']) || !/^https:\/\/([a-z0-9.-]+|\[[0-9a-f:]+\])(:[0-9]{1,5})?$/i.test(entrance.origin ?? '') ||
      typeof entrance.ca !== 'string' || !/^[A-Za-z0-9+/]{64,5460}={0,2}$/.test(entrance.ca) || entrance.ca.length % 4 ||
      typeof entrance.pin !== 'string' || !/^[0-9a-f]{64}$/.test(entrance.pin) || /^0+$/.test(entrance.pin)) fail('entrance');
  if (payment === undefined) return {entrance};
  const token = payment?.token;
  if (!only(payment, ['chain', 'contract', 'asset', 'token', 'rpc']) || payment.chain !== 8283 || !address(payment.contract) || !address(payment.asset) ||
      payment.contract === payment.asset || (payment.rpc !== undefined && !https(payment.rpc)) ||
      (token !== undefined && (!only(token, ['name', 'symbol', 'label', 'decimals', 'boardDecimals']) || typeof token.name !== 'string' ||
        typeof token.symbol !== 'string' || typeof token.label !== 'string' || !Number.isInteger(token.decimals)))) fail('payment');
  return {entrance, payment};
}

function inlineCustomerOrderBuild({types}, {build}) {
  const target = path.resolve(__dirname, '../src/merchant/customerEntranceDeployment.ts');
  const bound = {approvedCustomerEntrance: build?.entrance, approvedOrderPayment: build?.payment};
  return {
    visitor: {
      VariableDeclarator(declaration, state) {
        const name = declaration.node.id.name;
        if (state.filename === target && Object.hasOwn(bound, name) && bound[name] !== undefined)
          declaration.node.init = types.valueToNode(bound[name]);
      },
    },
  };
}

module.exports = {customerOrderBuild, inlineCustomerOrderBuild};
