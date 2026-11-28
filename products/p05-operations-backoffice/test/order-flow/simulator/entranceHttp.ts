// Stands in for CommerceHttp.customerEntrance (packages/protocol/platform/android/com/nu54/protocol/CommerceHttp.kt)
// in scripts/simulate_user_app.py: the same checks of what is sent, the same trust and the same answer, with the
// request made from the host process instead of the phone. entranceHttp.test.ts holds it against a local server.
//
// The same: which paths, bodies and session tokens are let out; TLS 1.3 without a client certificate; a server
// accepted only when its chain ends at the anchor's CA, its name is the origin's and its leaf carries a pinned key
// (the system's certificate store is never asked); no redirect followed; the answers that are handed over, their
// size and the Retry-After. Stood in for: the phone's own network path, Android's TLS provider and its timeouts on a
// mobile network.
import {createHash, X509Certificate} from 'node:crypto';
import * as https from 'node:https';
import {checkServerIdentity} from 'node:tls';

const reference = '[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}';
const storeList = new RegExp(`^/v2/customer/stores\\?after=${reference}$`);
const storeRead = new RegExp(`^/v2/customer/stores/${reference}(/menu/(0|[1-9][0-9]{0,4})|/photos/[0-9a-f]{64}/(0|[1-9][0-9]{0,5}))?$`);
const answers = [200, 400, 404, 409, 429, 503];

/** The module rejects with these codes (CommerceHttpModule.kt): a failed check, or anything else that went wrong. */
const failure = (code: 'COMMERCE_REFUSED' | 'COMMERCE_UNAVAILABLE') => Object.assign(new Error('The enrolled commerce connection is unavailable'), {code});
function need(holds: boolean) { if (!holds) throw failure('COMMERCE_REFUSED'); }

export type EntranceCall = {path: string; operation?: string; status?: number; retryAfter?: number; failed?: string};

/**
 * One request to the customer entrance, as the native module makes it. Answers `{"status":n,"body":"…"}` and
 * `"retryAfter":seconds` when the server sent a Retry-After of whole seconds.
 */
export async function customerEntrance(anchor: string, path: string, session: string, body: string): Promise<string> {
  const read = path === '/v2/customer/stores' || storeList.test(path) || storeRead.test(path);
  const commerce = path === '/v2/customer/commerce', cancel = path === '/v2/customer/checkout/cancel';
  need(read || commerce || cancel || path === '/v2/customer/challenge' || path === '/v2/customer/proof');
  const payload = Buffer.from(body, 'utf8');
  need(read ? payload.length === 0 : payload.length >= 2 && payload.length <= (commerce ? 8192 : cancel ? 256 : 1024));
  need(commerce || cancel ? /^[0-9a-f]{64}$/.test(session) : session === '');
  need(Buffer.byteLength(anchor) >= 1 && Buffer.byteLength(anchor) <= 8192);
  let trust: {origin?: unknown; ca?: unknown; pin?: unknown}, base: URL, ca: X509Certificate;
  try {
    trust = JSON.parse(anchor);
    base = new URL(String(trust.origin));
    ca = new X509Certificate(Buffer.from(String(trust.ca), 'base64'));
  } catch {
    throw failure('COMMERCE_UNAVAILABLE');
  }
  // An origin and nothing more: no user, no path, no query, no fragment.
  need(typeof trust.origin === 'string' && /^https:\/\/[^/?#@]+$/.test(trust.origin) && base.protocol === 'https:' && !!base.hostname);
  const pins = typeof trust.pin === 'string' ? [trust.pin] : Array.isArray(trust.pin) ? trust.pin : [];
  need(pins.length >= 1 && pins.length <= 2 && new Set(pins).size === pins.length && pins.every(pin => typeof pin === 'string' && /^[0-9a-f]{64}$/.test(pin)));
  const limit = read || commerce ? 32768 : 8192;
  return new Promise<string>((resolve, reject) => {
    const call = https.request({
      host: base.hostname.replace(/^\[|\]$/g, ''), port: base.port || 443, path, method: read ? 'GET' : 'POST', agent: false,
      // Only the anchor's CA is trusted, and no key is offered: a server that asks for a certificate gets none.
      ca: ca.toString(), minVersion: 'TLSv1.3', maxVersion: 'TLSv1.3', timeout: 7000,
      checkServerIdentity(name, peer) {
        const named = checkServerIdentity(name, peer);
        if (named) return named;
        const key = new X509Certificate(peer.raw).publicKey.export({type: 'spki', format: 'der'});
        return pins.includes(createHash('sha256').update(key).digest('hex')) ? undefined : new Error('wrong commerce pin');
      },
      headers: {'Content-Type': 'application/json', 'Cache-Control': 'no-store', Connection: 'close',
        ...(commerce || cancel ? {Authorization: `Bearer ${session}`} : {}), ...(read ? {} : {'Content-Length': payload.length})},
    }, reply => {
      const status = reply.statusCode ?? 0;
      // What the entrance answers with. A redirect or anything else is not an answer.
      if (!answers.includes(status)) { reply.destroy(); reject(failure('COMMERCE_REFUSED')); return; }
      const parts: Buffer[] = [];
      let size = 0;
      reply.on('data', (part: Buffer) => {
        size += part.length;
        if (size > limit) { reply.destroy(); reject(failure('COMMERCE_REFUSED')); return; }
        parts.push(part);
      }).on('end', () => {
        const wait = reply.headers['retry-after'];
        resolve(JSON.stringify({status, body: Buffer.concat(parts).toString('utf8'),
          ...(typeof wait === 'string' && /^[1-9][0-9]{0,3}$/.test(wait) ? {retryAfter: Number(wait)} : {})}));
      }).on('error', () => reject(failure('COMMERCE_UNAVAILABLE')));
    });
    call.on('timeout', () => call.destroy(new Error('timeout')));
    call.on('error', () => reject(failure('COMMERCE_UNAVAILABLE')));
    call.end(read ? undefined : payload);
  });
}

/** What a request was, for the record of a run: the path without its store, and the command's operation. */
export function describeCall(path: string, body: string): EntranceCall {
  let operation: string | undefined;
  try { operation = (JSON.parse(body) as {command?: {operation?: string}}).command?.operation; } catch { /* not a command */ }
  return {path: path.replace(new RegExp(reference, 'g'), '…'), ...(operation ? {operation} : {})};
}
