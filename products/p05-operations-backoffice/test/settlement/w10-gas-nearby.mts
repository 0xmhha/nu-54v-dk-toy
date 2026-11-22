import assert from 'node:assert/strict';
import {randomBytes, randomUUID} from 'node:crypto';
import {readFileSync} from 'node:fs';
import {request} from 'node:https';
import {
  CommerceClient, NearbyCommerceLink, V2Journal, fromBase64, toBase64,
  unpackNearbyPresentation, verifyPresentedGasSponsor, preparePresentedPayment, verifyCommerceApproval, hexToBytes,
  type CommercePaymentProfile, type NearbyRequest,
} from '../../../../packages/protocol/ts/src/index.ts';
import {MerchantInbox} from '../../../p04-merchant-kiosk/src/kiosk/merchantInbox.ts';
import {NearbyRelay} from '../../../p04-merchant-kiosk/src/kiosk/nearbyRelay.ts';

const data = JSON.parse(readFileSync(0, 'utf8')) as {
  endpoint: string; ca: string; cert: string; key: string;
  tenant: string; terminal: string; branch: string; customer: string; sale: string;
  origin: string; signed: string; profile: CommercePaymentProfile; sponsored: boolean;
  forward?: {key: string; signature: string};
};
assert.equal(new URL(data.endpoint).hostname, '127.0.0.1');
const material = {ca:readFileSync(data.ca),cert:readFileSync(data.cert),key:readFileSync(data.key)};
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
await inbox.grantClaim(data.sale,data.customer,expiry);
const relay = new NearbyRelay(api,inbox,async () => randomBytes(16),() => undefined,() => {});
const bound = {
  kind:'ready' as const, link:'w10-synthetic', epoch:toBase64(randomBytes(16)), branch:data.branch, expiresAt:expiry
};
async function query(buyer: string, message: NearbyRequest = {op:'checkout',branch:data.branch,order:data.sale,key:data.sale}) {
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
const result = await query(data.customer);
assert.equal(result.status,'ok', trace.join(', '));
const {quote, order} = unpackNearbyPresentation(result.value,data.branch,fromBase64);
assert.equal(quote.signedQuote,data.signed);
const scope = {branch:data.branch,sale:data.sale,customer:data.customer,origin:data.origin};
if (data.sponsored) {
  assert(verifyPresentedGasSponsor(quote,order,data.profile,clock(),scope).account);
} else {
  assert.throws(() => verifyPresentedGasSponsor(quote,order,data.profile,clock(),scope), /GAS_SPONSOR_MISSING/);
}
const foreign = await query(randomUUID());
assert.equal(foreign.status,'denied');
assert.equal(foreign.value,undefined);
if (data.forward) {
  const payment = preparePresentedPayment(quote,order,data.profile,clock(),scope);
  const signature = verifyCommerceApproval(payment,hexToBytes(data.forward.signature),clock());
  const command = {op:'forward', branch:data.branch, order:data.sale, key:data.forward.key,
    quoteDigest:payment.digest, approvalDigest:payment.approvalDigest, signature} as const;
  const denied = await query(randomUUID(), command);
  assert.equal(denied.status,'denied');
  const accepted = await query(data.customer, command);
  assert.equal(accepted.status,'ok', trace.join(', '));
  assert.deepEqual(accepted.value,{submitted:data.sale});
}
console.log('W10 actual P04 C2 forwarded the original quote, retained customer scope, and enforced sponsorship:',data.sponsored);
