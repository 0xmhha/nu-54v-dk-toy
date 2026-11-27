const fs = require('node:fs');
const path = require('node:path');

const repo = path.resolve(__dirname, '../../..');
const source = relative => fs.readFileSync(path.join(repo, relative), 'utf8');

test('terminal startup journals are accepted by the iOS and Android terminal vaults', () => {
  const inbox = source('products/p04-merchant-kiosk/src/kiosk/merchantInbox.ts');
  const journals = [...inbox.matchAll(/^const (?:inboxKey|nearbyKey|denialKey) = '([^']+)';$/gm)]
    .map(([, name]) => name);
  expect(journals).toEqual(['commerce-inbox-v2', 'commerce-nearby-inbox-v2', 'commerce-denials-v2']);
  const ios = source('packages/protocol/platform/ios/CommerceHttp.h');
  const android = source('packages/protocol/platform/android/com/nu54/protocol/CommerceHttp.kt');
  for (const name of journals) {
    expect(ios).toContain(`@"${name}"`);
    expect(android).toContain(`"${name}"`);
  }
  const iosGuard = ios.slice(ios.indexOf('static BOOL journalName'), ios.indexOf('static NSMutableDictionary *profileQuery'));
  const androidGuard = android.slice(android.indexOf('private fun journalName'), android.indexOf('@Synchronized'));
  for (const name of journals.slice(1)) {
    expect(iosGuard).toMatch(new RegExp(`#ifndef NU54_TRAVEL_RECORDER[\\s\\S]*@"${name}"[\\s\\S]*#endif`));
    expect(androidGuard).toMatch(new RegExp(`"terminal" in slots && name in setOf\\([^\\n]*"${name}"`));
  }
});
