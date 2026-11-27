import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {journalAllowed, journalNameLimit, journalNames, journalPatterns, journalStore} from './fixtures/journal';

// The phone's sealed journal takes only the names its native module lists. The list is written three times: in
// Kotlin, in Objective-C and in the test journal. They are held against each other here, with the names the app builds.

const platform = resolve(__dirname, '../../../packages/protocol/platform');
const kotlin = readFileSync(resolve(platform, 'android/com/nu54/protocol/CommerceHttp.kt'), 'utf8');
const objc = readFileSync(resolve(platform, 'ios/CommerceHttp.h'), 'utf8');
const quoted = (list: string) => [...list.matchAll(/"([^"]+)"/g)].map(match => match[1]);
const hex = 'ab'.repeat(32);

test('the Android module lists the same names and patterns', () => {
  const body = /private fun journalName\(name:String\):String \{([\s\S]*?)\n {4}\}/.exec(kotlin)![1];
  expect(quoted(/require\(name in setOf\(([^)]*)\)/.exec(body)![1])).toEqual(journalNames);
  expect(/name\.matches\(Regex\("([^"]+)"\)\)/.exec(body)![1]).toBe(journalPatterns);
});

test('the iOS module lists the same names and patterns, ends each at the very end, and admits the longest name', () => {
  const body = /static BOOL journalName\(NSString \*name\) \{([\s\S]*?)\n\}/.exec(objc)![1];
  expect(quoted(/return \[@\[([^\]]*)\] containsObject:name\]/.exec(body)![1])).toEqual(journalNames);
  const [, limit, pattern] = /name\.length<=([0-9]+)\) && \[name rangeOfString:@"\^\(([^"]+)\)\\\\z"/.exec(body)!;
  expect(pattern).toBe(journalPatterns);
  expect(Number(limit)).toBe(journalNameLimit);
  expect(`customer-sessions-${hex}`).toHaveLength(journalNameLimit);
  // `$` also matches before a last line break; no pattern of this header may end with it.
  expect(objc).not.toMatch(/\$" options:NSRegularExpressionSearch/);
  expect(objc).not.toMatch(/NSString \*[a-z]+=@"\^[^"]*\$";/);
});

test('every name the app keeps something under is a name the device takes', () => {
  for (const name of [`customer-order-${hex}`, `customer-payment-${hex}`, `customer-qr-${hex}`, `customer-legacy-${hex}`, `customer-sessions-${hex}`,
    'customer-order-v2', 'customer-payment-v2', 'customer-qr-v2', 'customer-stores-v1', 'subscription-alerts-v2', 'market-lab-8283-v1',
    `travel-hold-${hex}`, 'travel-local-00000000-0000-4000-8000-000000000002', `cloud-p03-${hex}`])
    expect([name, journalAllowed(name)]).toEqual([name, true]);
  for (const name of [`customer-session-${hex}`, `customer-sessions-${hex}0`, `customer-sessions-${hex}\n`, `customer-sessions-${hex.toUpperCase()}`,
    `customer-stores-${hex}`, 'customer-sessions-v1', ''])
    expect([name, journalAllowed(name)]).toEqual([name, false]);
});

test('the test journal refuses a name the device refuses', async () => {
  const journal = journalStore();
  await journal.saveJournal(`customer-sessions-${hex}`, '[]');
  expect(await journal.loadJournal(`customer-sessions-${hex}`)).toBe('[]');
  await expect(journal.saveJournal(`customer-session-${hex}`, '[]')).rejects.toMatchObject({code: 'COMMERCE_REFUSED'});
  await expect(journal.loadJournal('orders')).rejects.toMatchObject({code: 'COMMERCE_REFUSED'});
});
