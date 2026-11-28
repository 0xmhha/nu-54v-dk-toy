// The stand-in for the phone's request to the customer entrance (entranceHttp.ts), held against a local TLS server:
// what CommerceHttp.customerEntrance of packages/protocol/platform/android lets out, whom it trusts and what it
// hands back. The CA, the server's key and a stranger's CA are made here with openssl and thrown away.
import {execFileSync} from 'node:child_process';
import {createHash, generateKeyPairSync, X509Certificate, type KeyObject} from 'node:crypto';
import {mkdtempSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import * as https from 'node:https';
import type {AddressInfo} from 'node:net';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import type {TLSSocket} from 'node:tls';
import {customerEntrance, describeCall} from './entranceHttp';

type Seen = {method: string; path: string; authorization: string; body: string; certificate: boolean; type: string};
type Answer = {status: number; body: string; headers?: Record<string, string>};
type Server = {origin: string; seen: Seen[]; answer: Answer; close: () => Promise<void>};

let folder: string;
const stores = '/v2/customer/stores', branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c39e0', session = '5e'.repeat(32);
const made: Server[] = [];

function openssl(...args: string[]) { execFileSync('openssl', args, {cwd: folder, stdio: ['ignore', 'ignore', 'pipe']}); }
const pem = (name: string) => readFileSync(join(folder, name), 'utf8');
const der = (name: string) => new X509Certificate(pem(name)).raw.toString('base64');
const pinOf = (key: KeyObject) => createHash('sha256').update(key.export({type: 'spki', format: 'der'})).digest('hex');
function key(name: string) {
  const pair = generateKeyPairSync('ec', {namedCurve: 'prime256v1'});
  writeFileSync(join(folder, name + '.key'), pair.privateKey.export({type: 'pkcs8', format: 'pem'}), {mode: 0o600});
  return pair.publicKey;
}
function authority(name: string) {
  key(name);
  openssl('req', '-new', '-x509', '-key', name + '.key', '-subj', '/CN=NU54 entrance test ' + name, '-days', '1', '-out', name + '.pem');
}
/** A server certificate for `host` under the CA `by`. */
function issue(name: string, by: string, host = 'IP:127.0.0.1') {
  const publicKey = key(name);
  openssl('req', '-new', '-key', name + '.key', '-subj', '/CN=127.0.0.1', '-out', name + '.csr');
  writeFileSync(join(folder, name + '.ext'), `basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=${host}\n`);
  openssl('x509', '-req', '-in', name + '.csr', '-CA', by + '.pem', '-CAkey', by + '.key', '-CAcreateserial', '-days', '1', '-extfile', name + '.ext', '-out', name + '.pem');
  return pinOf(publicKey);
}
/** A server that asks for a client certificate, to see that none comes, and records what it was sent. */
async function serve(name: string, more: https.ServerOptions = {}): Promise<Server> {
  const seen: Seen[] = [];
  const state: Server = {origin: '', seen, answer: {status: 200, body: '{"stores":[]}'}, close: async () => {}};
  const server = https.createServer({key: pem(name + '.key'), cert: pem(name + '.pem'), minVersion: 'TLSv1.3', requestCert: true, rejectUnauthorized: false, ...more},
    (request, reply) => {
      let body = '';
      request.setEncoding('utf8').on('data', text => { body += text; }).on('end', () => {
        seen.push({method: request.method!, path: request.url!, authorization: request.headers.authorization ?? '', body,
          certificate: Object.keys((request.socket as TLSSocket).getPeerCertificate() ?? {}).length > 0, type: request.headers['content-type'] ?? ''});
        reply.writeHead(state.answer.status, state.answer.headers).end(state.answer.body);
      });
    });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  state.origin = `https://127.0.0.1:${(server.address() as AddressInfo).port}`;
  state.close = () => new Promise<void>(resolve => { server.closeAllConnections(); server.close(() => resolve()); });
  made.push(state);
  return state;
}
let pin = '', server: Server;
const anchor = (change: object = {}) => JSON.stringify({origin: server.origin, ca: der('ca.pem'), pin, ...change});
const refusal = (code: string) => expect.objectContaining({code, message: 'The enrolled commerce connection is unavailable'});

beforeAll(async () => {
  folder = mkdtempSync(join(tmpdir(), 'nu54-entrance-'));
  authority('ca');
  authority('stranger');
  pin = issue('server', 'ca');
  server = await serve('server');
});
afterAll(async () => {
  await Promise.all(made.map(item => item.close()));
  rmSync(folder, {recursive: true, force: true});
});
beforeEach(() => { server.seen.length = 0; server.answer = {status: 200, body: '{"stores":[]}'}; });

test('a store read is a GET with no body, no session and no client certificate', async () => {
  for (const path of [stores, `${stores}?after=${branch}`, `${stores}/${branch}`, `${stores}/${branch}/menu/0`, `${stores}/${branch}/menu/12`,
    `${stores}/${branch}/photos/${'a1'.repeat(32)}/18000`]) {
    expect(JSON.parse(await customerEntrance(anchor(), path, '', ''))).toEqual({status: 200, body: '{"stores":[]}'});
  }
  expect(server.seen.map(item => item.path)).toEqual([stores, `${stores}?after=${branch}`, `${stores}/${branch}`, `${stores}/${branch}/menu/0`,
    `${stores}/${branch}/menu/12`, `${stores}/${branch}/photos/${'a1'.repeat(32)}/18000`]);
  for (const item of server.seen) expect(item).toMatchObject({method: 'GET', authorization: '', body: '', certificate: false});
});

test('a challenge and a proof are posted without a session; a session request and a cart taken back carry it as a bearer token', async () => {
  await customerEntrance(anchor(), '/v2/customer/challenge', '', '{"purpose":"account"}');
  await customerEntrance(anchor(), '/v2/customer/proof', '', '{"challenge":"c1"}');
  await customerEntrance(anchor(), '/v2/customer/commerce', session, '{"command":{"operation":"order.status"}}');
  await customerEntrance(anchor(), '/v2/customer/checkout/cancel', session, `{"saleRef":"${branch}"}`);
  expect(server.seen.map(({method, path, authorization, body, type}) => ({method, path, authorization, body, type}))).toEqual([
    {method: 'POST', path: '/v2/customer/challenge', authorization: '', body: '{"purpose":"account"}', type: 'application/json'},
    {method: 'POST', path: '/v2/customer/proof', authorization: '', body: '{"challenge":"c1"}', type: 'application/json'},
    {method: 'POST', path: '/v2/customer/commerce', authorization: 'Bearer ' + session, body: '{"command":{"operation":"order.status"}}', type: 'application/json'},
    {method: 'POST', path: '/v2/customer/checkout/cancel', authorization: 'Bearer ' + session, body: `{"saleRef":"${branch}"}`, type: 'application/json'},
  ]);
  expect(server.seen.every(item => !item.certificate)).toBe(true);
});

test('what the native module would not send is not sent', async () => {
  const refused: [string, string, string, string?][] = [
    ['/v2/customer/sessions', '', '{}'], ['/v2/commerce', '', '{}'], [`${stores}/${branch}/orders`, '', ''], [`${stores}?after=${branch}&x=1`, '', ''],
    [`${stores}?after=next`, '', ''], [`${stores}/${branch}/menu/01`, '', ''], [`${stores}/${branch.toUpperCase()}`, '', ''], [`${stores}/../operations`, '', ''],
    [stores, '', '{}'], [stores, session, ''], // a read carries nothing
    ['/v2/customer/challenge', '', ''], ['/v2/customer/challenge', '', '{'], ['/v2/customer/challenge', session, '{}'], ['/v2/customer/proof', '', 'x'.repeat(1025)],
    ['/v2/customer/commerce', '', '{}'], ['/v2/customer/commerce', 'short', '{}'], ['/v2/customer/commerce', session.toUpperCase(), '{}'],
    ['/v2/customer/commerce', session, 'x'.repeat(8193)], ['/v2/customer/checkout/cancel', session, 'x'.repeat(257)], ['/v2/customer/checkout/cancel', '', '{}'],
    // An anchor that is not an https origin with one or two different pinned keys.
    [stores, '', '', anchor({origin: server.origin.replace('https', 'http')})], [stores, '', '', anchor({origin: server.origin + '/'})],
    [stores, '', '', anchor({origin: server.origin + '/v2'})], [stores, '', '', anchor({origin: server.origin.replace('//', '//user@')})],
    [stores, '', '', anchor({pin: []})], [stores, '', '', anchor({pin: [pin, pin]})], [stores, '', '', anchor({pin: [pin, 'cd'.repeat(32), 'ef'.repeat(32)]})],
    [stores, '', '', anchor({pin: pin.toUpperCase()})], [stores, '', '', anchor({pin: 7})], [stores, '', '', 'x'.repeat(8193)],
  ];
  for (const [path, token, body, trust] of refused) {
    if (path === '/v2/customer/challenge' && body === '{') continue; // two bytes: the server decides what a body means
    await expect(customerEntrance(trust ?? anchor(), path, token, body)).rejects.toEqual(refusal('COMMERCE_REFUSED'));
  }
  // An anchor that does not read at all is the module's other failure.
  for (const trust of ['', 'not json', anchor({ca: 'AAAA'}), anchor({origin: 'store'})])
    await expect(customerEntrance(trust, stores, '', '')).rejects.toEqual(trust === '' ? refusal('COMMERCE_REFUSED') : refusal('COMMERCE_UNAVAILABLE'));
  expect(server.seen).toEqual([]);
});

test('the largest bodies the module sends go out', async () => {
  await customerEntrance(anchor(), '/v2/customer/proof', '', 'x'.repeat(1024));
  await customerEntrance(anchor(), '/v2/customer/commerce', session, 'x'.repeat(8192));
  await customerEntrance(anchor(), '/v2/customer/checkout/cancel', session, 'x'.repeat(256));
  expect(server.seen.map(item => item.body.length)).toEqual([1024, 8192, 256]);
});

test('only a server under the anchor\'s CA with a pinned key and the origin\'s name is spoken to', async () => {
  const stranger = await serve((issue('unknown', 'stranger'), 'unknown'));
  const otherKey = await serve((issue('rotated', 'ca'), 'rotated'));
  const otherName = await serve((issue('named', 'ca', 'DNS:store.example'), 'named'));
  const trust = (to: Server, change: object = {}) => JSON.stringify({origin: to.origin, ca: der('ca.pem'), pin, ...change});
  // Another CA, though the pin is the server's own key; the right CA with a key that is not pinned; a name that is not the origin's.
  await expect(customerEntrance(trust(stranger, {pin: pinOf(new X509Certificate(pem('unknown.pem')).publicKey)}), stores, '', '')).rejects.toEqual(refusal('COMMERCE_UNAVAILABLE'));
  await expect(customerEntrance(trust(otherKey), stores, '', '')).rejects.toEqual(refusal('COMMERCE_UNAVAILABLE'));
  await expect(customerEntrance(trust(otherName, {pin: pinOf(new X509Certificate(pem('named.pem')).publicKey)}), stores, '', '')).rejects.toEqual(refusal('COMMERCE_UNAVAILABLE'));
  // The system's certificate store plays no part: the stranger's own CA as the anchor is what makes it trusted.
  expect(JSON.parse(await customerEntrance(JSON.stringify({origin: stranger.origin, ca: der('stranger.pem'),
    pin: pinOf(new X509Certificate(pem('unknown.pem')).publicKey)}), stores, '', '')).status).toBe(200);
  // While the server changes its key the build pins two: the current one and the next.
  const next = pinOf(new X509Certificate(pem('rotated.pem')).publicKey);
  expect(JSON.parse(await customerEntrance(trust(otherKey, {pin: [pin, next]}), stores, '', '')).status).toBe(200);
  expect(JSON.parse(await customerEntrance(anchor({pin: [pin, next]}), stores, '', '')).status).toBe(200);
  expect([stranger.seen.length, otherKey.seen.length, otherName.seen.length]).toEqual([1, 1, 0]);
});

test('a server that speaks TLS 1.2 only, or does not answer, is no answer', async () => {
  const old = await serve('server', {minVersion: 'TLSv1.2', maxVersion: 'TLSv1.2'});
  await expect(customerEntrance(JSON.stringify({origin: old.origin, ca: der('ca.pem'), pin}), stores, '', '')).rejects.toEqual(refusal('COMMERCE_UNAVAILABLE'));
  expect(old.seen).toEqual([]);
  const gone = await serve('server');
  await gone.close();
  await expect(customerEntrance(JSON.stringify({origin: gone.origin, ca: der('ca.pem'), pin}), stores, '', '')).rejects.toEqual(refusal('COMMERCE_UNAVAILABLE'));
});

test('the answers of the entrance are handed over with their status, and a Retry-After of whole seconds', async () => {
  for (const status of [200, 400, 404, 409, 429, 503]) {
    server.answer = {status, body: `{"status":${status}}`};
    expect(JSON.parse(await customerEntrance(anchor(), '/v2/customer/proof', '', '{}'))).toEqual({status, body: `{"status":${status}}`});
  }
  for (const [wait, kept] of [['3', 3], ['10', 10], ['9999', 9999], ['0', undefined], ['03', undefined], ['10000', undefined], ['soon', undefined],
    ['Wed, 21 Oct 2026 07:28:00 GMT', undefined]] as const) {
    server.answer = {status: 429, body: '{"error":"BUSY"}', headers: {'Retry-After': wait}};
    expect(JSON.parse(await customerEntrance(anchor(), stores, '', ''))).toEqual({status: 429, body: '{"error":"BUSY"}', ...(kept ? {retryAfter: kept} : {})});
  }
  // A plain not found has a body that is not JSON; it comes back as it is.
  server.answer = {status: 404, body: '404 page not found\n'};
  expect(JSON.parse(await customerEntrance(anchor(), `${stores}/${branch}`, '', ''))).toEqual({status: 404, body: '404 page not found\n'});
});

test('a redirect, another status or an answer that is too long is not an answer', async () => {
  for (const answer of [{status: 302, body: '', headers: {Location: server.origin + stores}}, {status: 201, body: '{}'}, {status: 401, body: '{}'},
    {status: 500, body: '{}'}]) {
    server.answer = answer;
    await expect(customerEntrance(anchor(), stores, '', '')).rejects.toEqual(refusal('COMMERCE_REFUSED'));
  }
  expect(server.seen.map(item => item.path)).toEqual([stores, stores, stores, stores]); // the redirect was not followed
  // A store read and a session answer may hold 32,768 bytes; a challenge, a proof and a cancel 8,192.
  server.answer = {status: 200, body: 'x'.repeat(32768)};
  expect(JSON.parse(await customerEntrance(anchor(), stores, '', '')).body).toHaveLength(32768);
  expect(JSON.parse(await customerEntrance(anchor(), '/v2/customer/commerce', session, '{}')).body).toHaveLength(32768);
  await expect(customerEntrance(anchor(), '/v2/customer/proof', '', '{}')).rejects.toEqual(refusal('COMMERCE_REFUSED'));
  server.answer = {status: 200, body: 'x'.repeat(32769)};
  await expect(customerEntrance(anchor(), stores, '', '')).rejects.toEqual(refusal('COMMERCE_REFUSED'));
  server.answer = {status: 200, body: 'x'.repeat(8192)};
  expect(JSON.parse(await customerEntrance(anchor(), '/v2/customer/checkout/cancel', session, '{}')).body).toHaveLength(8192);
});

test('a request is written down without the store it names', () => {
  expect(describeCall(`${stores}/${branch}/menu/0`, '')).toEqual({path: '/v2/customer/stores/…/menu/0'});
  expect(describeCall('/v2/customer/commerce', '{"command":{"operation":"order.create"}}')).toEqual({path: '/v2/customer/commerce', operation: 'order.create'});
  expect(describeCall('/v2/customer/proof', 'not json')).toEqual({path: '/v2/customer/proof'});
});
