import {readdirSync, readFileSync, statSync} from 'node:fs';
import {join, resolve} from 'node:path';

// What the phone may ask the customer entrance for is written twice in the native modules, in Kotlin and in
// Objective-C, and once on the server (opsd RegisterCustomer). The two native lists are read here and held against the
// routes the app sends, so a route the app uses cannot be missing on one platform and a route it must not use cannot
// be open on either.

const platform = resolve(__dirname, '../../../packages/protocol/platform');
const kotlin = readFileSync(resolve(platform, 'android/com/nu54/protocol/CommerceHttp.kt'), 'utf8');
const objc = readFileSync(resolve(platform, 'ios/CommerceHttp.h'), 'utf8');
const kotlinEntrance = /fun customerEntrance\(anchor:String, path:String, session:String, body:String\):String \{([\s\S]*?)\n {4}\}/.exec(kotlin)![1];
const objcEntrance = /- \(void\)customerRequest:\(NSString \*\)anchor path:([\s\S]*?)\n\}\n#endif/.exec(objc)![1];

const store = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', photo = 'ab'.repeat(32);

/** The paths each native module takes: the ones it names outright and the patterns it matches a whole path against. */
function allowed(source: string, patterns: RegExp[], literals: string[]) {
  return (path: string) => literals.includes(path) || patterns.some(pattern => pattern.test(path));
}
const kotlinReference = /val reference="([^"]+)"/.exec(kotlinEntrance)![1];
// Kotlin `Regex("…").matches(path)` is a match of the whole path; `\\` in the source is one backslash.
const kotlinPatterns = [...kotlinEntrance.matchAll(/Regex\("(\/v2\/customer\/[^"]+)"\)\.matches\(path\)/g)]
  .map(match => new RegExp(`^(?:${match[1].replace(/\\\\/g, '\\').replace(/\$reference/g, kotlinReference)})$`));
const kotlinLiterals = [...kotlinEntrance.matchAll(/"(\/v2\/customer\/[a-z/]+)"/g)].map(match => match[1]);
// Objective-C patterns are anchored by `^` and `\z` (the very end); `$` would also match before a last line break.
const objcPatterns = [...objcEntrance.matchAll(/rangeOfString:@"(\^\/v2\/customer\/[^"]+)\\\\z" options:NSRegularExpressionSearch/g)]
  .map(match => new RegExp(`${match[1].replace(/\\\\/g, '\\')}$`));
const objcLiterals = [...objcEntrance.matchAll(/isEqual:@"(\/v2\/customer\/[a-z/]+)"|@"(\/v2\/customer\/[a-z/]+)"[,\]]/g)].map(match => match[1] ?? match[2]);
const android = allowed(kotlinEntrance, kotlinPatterns, kotlinLiterals), ios = allowed(objcEntrance, objcPatterns, objcLiterals);

const sent = ['/v2/customer/challenge', '/v2/customer/proof', '/v2/customer/commerce', '/v2/customer/checkout/cancel', '/v2/customer/stores',
  `/v2/customer/stores?after=${store}`, `/v2/customer/stores/${store}`, `/v2/customer/stores/${store}/menu/0`, `/v2/customer/stores/${store}/menu/12`,
  `/v2/customer/stores/${store}/photos/${photo}/0`, `/v2/customer/stores/${store}/photos/${photo}/18000`];
const refused = ['/v2/customer/stores?', '/v2/customer/stores?after=', `/v2/customer/stores?after=${store}&limit=1000`, `/v2/customer/stores?limit=1&after=${store}`,
  `/v2/customer/stores?after=${store.toUpperCase()}`, `/v2/customer/stores?after=${store}\n`, `/v2/customer/stores?after=${store}#x`, `/v2/customer/stores?before=${store}`,
  `/v2/customer/stores/${store}?after=${store}`, `/v2/customer/stores/${store}/menu/0?page=1`, `/v2/customer/stores/${store}/menu/01`,
  '/v2/customer/checkout/cancel/', '/v2/customer/checkout/cancel?saleRef=x', '/v2/customer/checkout', '/v2/customer/commerce?x=1', '/v2/commerce',
  `/v2/operations/pickup-scan/${store}/${store}`, `/v2/operations/store-page/${store}`, '/healthz', '/v2/customer/../operations', ''];

test('both native modules read the same lists from their sources', () => {
  expect(kotlinPatterns).toHaveLength(2);
  expect(objcPatterns).toHaveLength(2);
  expect([...new Set(kotlinLiterals)].sort()).toEqual(['/v2/customer/challenge', '/v2/customer/checkout/cancel', '/v2/customer/commerce', '/v2/customer/proof', '/v2/customer/stores']);
  expect([...new Set(objcLiterals)].sort()).toEqual([...new Set(kotlinLiterals)].sort());
});

test.each(sent)('Android and iOS take %s', path => {
  expect([android(path), ios(path)]).toEqual([true, true]);
});

test.each(refused)('Android and iOS refuse %j', path => {
  expect([android(path), ios(path)]).toEqual([false, false]);
});

test('the one query either module takes is the store list\'s place to go on from, a store reference', () => {
  expect(kotlinEntrance).toContain('Regex("/v2/customer/stores\\\\?after=$reference").matches(path)');
  expect(objcEntrance).toContain('url.percentEncodedQuery=[@"after=" stringByAppendingString:after]');
  // On iOS the query is set apart from the path, so the request goes to the list and not to a path named `stores?after=…`.
  expect(objcEntrance).toContain('url.percentEncodedPath=after ? @"/v2/customer/stores" : path;');
});

test('a session token goes with a C3 request and with a cart taken back, and with nothing else', () => {
  expect(kotlinEntrance).toContain('require(if(commerce || cancel) Regex("[0-9a-f]{64}").matches(session) else session.isEmpty())');
  expect(kotlinEntrance).toContain('if(commerce || cancel) connection.setRequestProperty("Authorization","Bearer $session")');
  expect(objcEntrance).toContain('BOOL held=(commerce || cancel) ? (session.length==64 && [session rangeOfCharacterFromSet:notHex].location==NSNotFound) : session.length==0;');
  expect(objcEntrance).toContain('if (commerce || cancel) [request setValue:[@"Bearer " stringByAppendingString:session] forHTTPHeaderField:@"Authorization"];');
  // The body of a cart taken back is one order reference: the server reads 256 bytes of it at most.
  expect(kotlinEntrance).toContain('payload.size in 2..(if(commerce) 8192 else if(cancel) 256 else 1024)');
  expect(objcEntrance).toContain('payload.length<=(commerce ? 8192 : cancel ? 256 : 1024)');
});

test('both modules hand back the Retry-After of an answer as whole seconds, under the same name', () => {
  expect(kotlinEntrance).toContain('connection.getHeaderField("Retry-After")');
  expect(kotlinEntrance).toMatch(/Regex\("\[1-9\]\[0-9\]\{0,3\}"\)\.matches\(wait\)\) reply\.put\("retryAfter",wait\.toInt\(\)\)/);
  expect(objc).toContain('valueForHTTPHeaderField:@"Retry-After"]');
  expect(objc).toMatch(/rangeOfString:@"\^\[1-9\]\[0-9\]\{0,3\}\\\\z" options:NSRegularExpressionSearch\]\.location!=NSNotFound\) self\.retryAfter=@\(wait\.integerValue\);/);
  expect(objc).toContain('if (self.retryAfter) answer[@"retryAfter"]=self.retryAfter;');
  // Both accept the two statuses a wait comes with.
  expect(kotlinEntrance).toContain('require(status in setOf(200,400,404,409,429,503))');
  expect(objc).toContain('BOOL answer=code==200 || code==400 || code==404 || code==409 || code==429 || code==503;');
});

test('nothing in the phone app or the kiosk asks for a pickup QR scan: only a store operator scans one', () => {
  const files = (dir: string): string[] => readdirSync(dir).flatMap(name => {
    const path = join(dir, name);
    if (['node_modules', 'Pods', 'build', '.gradle', 'third_party'].includes(name)) return [];
    return statSync(path).isDirectory() ? files(path) : /\.(tsx?|kt|mm?|h|swift|java)$/.test(name) ? [path] : [];
  });
  const sources = [resolve(__dirname, '../src'), resolve(__dirname, '../android/app/src'), resolve(__dirname, '../ios/NU54Renter'),
    resolve(__dirname, '../../p04-merchant-kiosk/src'), resolve(__dirname, '../../p04-merchant-kiosk/android/app/src'), platform].flatMap(files);
  expect(sources.length).toBeGreaterThan(100);
  const asking = sources.filter(path => /pickup-scan|pickupScan|scan_pickup/.test(readFileSync(path, 'utf8')));
  expect(asking).toEqual([]);
});
