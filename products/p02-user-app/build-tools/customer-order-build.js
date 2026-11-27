const fs = require('node:fs');
const path = require('node:path');

// Binds what a phone trusts for ordering without a store registration (src/merchant/customerEntranceDeployment.ts):
// the customer entrance of the store server and the order contract. NU54_CUSTOMER_ORDER_BUILD names a JSON file
//   {"entrance": {"origin": "https://…", "ca": "<DER in base64>", "pin": "<64 hex>" or ["<current>", "<next>"]},
//    "payment":  {"chain": 8283, "contract": "0x…", "asset": "0x…", "token": {…}, "rpc": "https://…"}}
// `payment` is optional (a build that lists stores but takes no order) and needs `entrance`. Unset, both stay
// undefined, which is what the repository ships.
//
// The app checks the same values again when it starts (customerEntrance.ts entranceAnchor, orderPaymentTrust.ts
// orderPaymentPin) and takes a value it refuses as no value: no store list, or no order. So every rule of the app
// is a rule here, and a value the app would refuse stops the bundle instead. test/customerOrderBuild.test.js holds
// the two sides against each other.

const address = value => typeof value === 'string' && /^0x[0-9a-f]{40}$/.test(value) && !/^0x0+$/.test(value);
const https = value => typeof value === 'string' && value.length <= 512 && /^https:\/\/[A-Za-z0-9.[\]:_-]+(?:\/[A-Za-z0-9/_.-]*)?$/.test(value);
const only = (value, names) => !!value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).every(name => names.includes(name));
const fail = part => { throw new Error(`NU54_CUSTOMER_ORDER_BUILD: ${part} is not a value a build may bind`); };

// The origin as travel/catalog.ts publicOrigin takes it, without a path.
function origin(value) {
  const parts = typeof value === 'string' &&
    /^https:\/\/((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?|\[[a-f0-9:]+\])(?::([0-9]{1,5}))?$/i.exec(value);
  return !!parts && (!parts[2] || (Number(parts[2]) >= 1 && Number(parts[2]) <= 65535));
}
const pinned = value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value) && !/^0+$/.test(value);
// One pinned key, or the current one and the next.
const pins = value => pinned(value) || (Array.isArray(value) && value.length >= 1 && value.length <= 2 && value.every(pinned) && new Set(value).size === value.length);

// A store's token as @nu54/protocol validPaymentToken and paymentProfile take it (commerce-payment.ts).
const tokenWord = /^[A-Za-z][A-Za-z0-9]{0,15}$/;
const tokenName = /^[\x21-\x7e](?:[\x20-\x7e]{0,62}[\x21-\x7e])?$/;
const coin = {asset: '0x' + '1000'.padStart(40, '0'), symbol: 'WKRC', label: 'KRWC'};
const dollar = {symbol: 'tUSDC', label: 'TestUSDC'};
function token(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const {boardDecimals: board, ...plain} = value;
  return Object.keys(plain).sort().join() === 'decimals,label,name,symbol' &&
    typeof value.name === 'string' && tokenName.test(value.name) && typeof value.symbol === 'string' && tokenWord.test(value.symbol) &&
    typeof value.label === 'string' && tokenWord.test(value.label) && Number.isInteger(value.decimals) && value.decimals >= 1 && value.decimals <= 18 &&
    (value.decimals > 8 ? Number.isInteger(board) && board >= 2 && board <= 8 : !('boardDecimals' in value));
}

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
  if (!only(entrance, ['origin', 'ca', 'pin']) || !origin(entrance.origin) ||
      typeof entrance.ca !== 'string' || !/^[A-Za-z0-9+/]{64,5460}={0,2}$/.test(entrance.ca) || entrance.ca.length % 4 || !pins(entrance.pin)) fail('entrance');
  if (payment === undefined) return {entrance};
  if (!only(payment, ['chain', 'contract', 'asset', 'token', 'rpc']) || payment.chain !== 8283 || !address(payment.contract) || !address(payment.asset) ||
      payment.contract === payment.asset || (payment.rpc !== undefined && !https(payment.rpc)) || (payment.token !== undefined && !token(payment.token))) fail('payment');
  // StableNet's own coin is at one address and no other token is; a build that names one without the other shows a wrong name.
  const shown = payment.token ?? dollar;
  if ((shown.symbol === coin.symbol) !== (payment.asset === coin.asset) || (shown.symbol === coin.symbol) !== (shown.label === coin.label)) fail('payment');
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
