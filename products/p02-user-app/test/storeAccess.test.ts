import {customerSubject} from '../src/merchant/storeAccess';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: null}));

describe('the customer of a board account', () => {
  test('is the subject the store server derives from the same account', () => {
    // opsd customer_entrance_test.go TestCustomerSubject holds the same value.
    expect(customerSubject('0x' + '1'.repeat(40))).toBe('1a07c82c-ec0d-4eca-98d2-6aec9f1bb156');
    expect(customerSubject('0x' + '2'.repeat(40))).not.toBe(customerSubject('0x' + '1'.repeat(40)));
    expect(customerSubject('0x' + 'ab'.repeat(20))).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  test('an account that is not one has no customer', () => {
    for (const bad of ['', '0x' + '1'.repeat(39), '0X' + '1'.repeat(40), '0x' + 'AB'.repeat(20), '1'.repeat(40)])
      expect(() => customerSubject(bad)).toThrow('CUSTOMER_ACCOUNT');
  });
});
