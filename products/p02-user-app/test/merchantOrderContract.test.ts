import { parseLabel, parseOrderLocator } from '../src/qr';
import { encodeOrderLocator } from '@nu54/protocol';

const ref = {
  branchRef: 'f0551c76-7592-4850-a414-54464d24dafe',
  orderRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea',
  expiresAt: 1050,
};
const raw = `nu54://order?v=2&branch=${ref.branchRef}&order=${ref.orderRef}&expires=${ref.expiresAt}`;

test('P02 reads only a bounded order locator from a distinct QR profile', () => {
  expect(encodeOrderLocator(ref, 1000)).toBe(raw);
  expect(parseOrderLocator(raw, 1000)).toEqual(ref);
  expect(() => parseOrderLocator(raw + '&amount=1', 1000)).toThrow();
  expect(() => parseOrderLocator(raw + '&payer=0x123', 1000)).toThrow();
  expect(() => parseOrderLocator(raw + '&order=other', 1000)).toThrow();
  expect(() => parseOrderLocator(raw, 1050)).toThrow();
  expect(() => parseOrderLocator(raw, 989)).toThrow();
  expect(() => parseOrderLocator(raw + '\n', 1000)).toThrow();
  expect(() =>
    parseOrderLocator(
      'nu54://bond?addr=AA:BB:CC:DD:EE:FF&passkey=012345',
      1000,
    ),
  ).toThrow();
  expect(() => parseLabel(raw)).toThrow();
});
