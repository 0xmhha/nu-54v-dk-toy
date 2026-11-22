import assert from 'node:assert/strict';
import {randomBytes, randomUUID} from 'node:crypto';
import {readFileSync} from 'node:fs';
import {request} from 'node:https';
import {
  CommerceClient, NearbyCommerceLink, V2Journal, fromBase64, toBase64,
  unpackNearbyPresentation, preparePresentedPayment, verifyCommerceApproval, hexToBytes,
  type CommercePaymentProfile, type NearbyRequest,
} from '../../../../packages/protocol/ts/src/index.ts';
import {MerchantInbox} from '../../../p04-merchant-kiosk/src/kiosk/merchantInbox.ts';
import {NearbyRelay} from '../../../p04-merchant-kiosk/src/kiosk/nearbyRelay.ts';

const data = JSON.parse(readFileSync(0, 'utf8')) as {
  endpoint: string; ca: string; cert: string; key: string;
  tenant: string; terminal: string; branch: string; customer: string; otherCustomer: string; sale: string;
  origin: string; reserveKey: string; profile: CommercePaymentProfile;
  forward?: {key: string; signature: string};
};
assert.equal(new URL(data.endpoint).hostname, '127.0.0.1');
const material = {ca:data.ca,cert:data.cert,key:data.key};
const clock = () => Math.floor(Date.now() / 1000);
const expiry = clock() + 60;
const trace: string[] = [];
const api = new CommerceClient({
  role:'terminal', tenantRef:data.tenant, branchRef:data.branch, subjectRef:data.terminal, expiresAt:expiry,
}, command => new Promise((resolve, reject) => {
  const operation = JSON.parse(command).command.operation as string;
  const connection = request(data.endpoint,{
    ...material, method:'POST', headers:{'Content-Type':'application/json'}
  }, incoming => {
    let raw = '';
    incoming.setEncoding('utf8');
    incoming.on('data', chunk => {raw += chunk;});
    incoming.on('end', () => {trace.push(operation + ':' + incoming.statusCode); resolve(raw);});
    incoming.on('error', reject);
  });
  connection.setTimeout(4000, () => connection.destroy(new Error('relay timeout')));
  connection.on('error', (error: NodeJS.ErrnoException) => {
    trace.push(operation + ':' + (error.code ?? 'transport error'));
    reject(error);
  });
  connection.end(command);
}), async () => randomUUID());

const custody = new Map<string,string>();
const inbox = await MerchantInbox.open({
  getSetting:async key => custody.get(key) ?? '',
  putSetting:async (key, value) => {custody.set(key,value);},
}, data.branch, data.terminal);
const relay = new NearbyRelay(api,inbox,async () => randomBytes(16),() => undefined,() => {});
const bound = {
  kind:'ready' as const, link:'k05-loopback-c2', epoch:toBase64(randomBytes(16)), branch:data.branch, expiresAt:expiry
};
async function query(buyer: string, message: NearbyRequest) {
  const terminal = new NearbyCommerceLink({...bound,peer:'a'.repeat(64),principal:buyer},'terminal',{
    async write(_, body) {customer.receive(body);},
    async close() {},
  },new V2Journal(10),(message, peer) => relay.handle(message,peer));
  const customer = new NearbyCommerceLink({...bound,peer:'b'.repeat(64),principal:data.terminal},'customer',{
    async write(_, body) {terminal.receive(body);},
    async close() {},
  },new V2Journal(10));
  try {
    return await customer.request(message,randomBytes(16));
  } finally {
    customer.close();
    terminal.close();
  }
}

let sale = data.sale;
if (!sale) {
  const cart = {origin:'menu',originRef:data.origin,menuVersion:1,
    items:[{sku:'near-cafe',option_key:'plain',quantity:1}]} as const;
  const reservation = await query(data.customer,{op:'reserve',branch:data.branch,key:data.reserveKey,
    deadline:expiry,asset:data.profile.asset,cart:{...cart,items:[...cart.items]}});
  assert.equal(reservation.status,'ok',trace.join(', '));
  assert(reservation.value && typeof reservation.value === 'object' && 'id' in reservation.value);
  assert.equal(typeof reservation.value.id,'string');
  sale = reservation.value.id as string;
  const status = {op:'status',branch:data.branch,order:sale,key:data.reserveKey,authoritative:true} as const;
  assert.equal((await query(data.customer,status)).status,'ok');
  assert.equal((await query(data.otherCustomer,status)).status,'denied');
}
const checkout = {op:'checkout',branch:data.branch,order:sale,key:data.reserveKey,authoritative:true} as const;
const result = await query(data.customer,checkout);
assert.equal(result.status,'ok',trace.join(', '));
const {quote, order} = unpackNearbyPresentation(result.value,data.branch,fromBase64);
assert.equal(order.id,sale);
const foreign = await query(data.otherCustomer,checkout);
assert.deepEqual(foreign,{status:'denied'});
const payment = preparePresentedPayment(quote,order,data.profile,clock(),{branch:data.branch,sale,
  customer:data.customer,origin:data.origin});
if (data.forward) {
  const signature = verifyCommerceApproval(payment,hexToBytes(data.forward.signature),clock());
  const command = {op:'forward',branch:data.branch,order:sale,key:data.forward.key,
    quoteDigest:payment.digest,approvalDigest:payment.approvalDigest,signature,authoritative:true} as const;
  assert.deepEqual(await query(data.otherCustomer,command),{status:'denied'});
  const accepted = await query(data.customer,command);
  assert.equal(accepted.status,'ok',trace.join(', '));
  assert.deepEqual(accepted.value,{submitted:sale});
}
console.log(JSON.stringify(quote));
