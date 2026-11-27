import {configuredEntrance, CustomerEntrance, nativeEntrance} from '../src/merchant/customerEntrance';

const mockCustomerRequest = jest.fn<Promise<string>, [string, string, string]>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  customerRequest: (origin: string, path: string, body: string) => mockCustomerRequest(origin, path, body),
}}));

const answer = (status: unknown, body: unknown) => JSON.stringify({status, body});
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';

describe('customer entrance over the native commerce module', () => {
  beforeEach(() => mockCustomerRequest.mockReset());

  test('posts to the canonical https origin and hands back the status and the parsed body', async () => {
    const customerRequest = jest.fn(async (_origin: string, _path: string, _body: string) => answer(200, '{"ok":true}'));
    const post = nativeEntrance('https://Store.Example:8444/', {customerRequest});
    expect(await post('/v2/customer/challenge', '{"a":1}')).toEqual({status: 200, body: {ok: true}});
    expect(customerRequest.mock.calls).toEqual([['https://store.example:8444', '/v2/customer/challenge', '{"a":1}']]);
  });

  test('a refusal keeps its status, and a body that is not JSON reads as null', async () => {
    const replies = [answer(429, '{"status":"error","error":"BUSY"}'), answer(404, '404 page not found\n'), answer(503, '')];
    const post = nativeEntrance('https://store.example', {customerRequest: async () => replies.shift()!});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 429, body: {status: 'error', error: 'BUSY'}});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 404, body: null});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 503, body: null});
  });

  test('no native method, a native failure and a malformed reply are all unavailable', async () => {
    const failing = {customerRequest: async () => { throw Object.assign(new Error('x'), {code: 'COMMERCE_UNAVAILABLE'}); }};
    const cases = [null, {}, failing, {customerRequest: async () => 'not json'}, {customerRequest: async () => 'null'},
      {customerRequest: async () => answer('200', '{}')}, {customerRequest: async () => answer(200, {ok: true})}];
    for (const http of cases)
      await expect(nativeEntrance('https://store.example', http)('/v2/customer/proof', '{}')).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
  });

  test('an origin that is not https never reaches the native module', async () => {
    const customerRequest = jest.fn(async () => answer(200, '{}'));
    for (const origin of ['http://store.example', 'https://store.example/path', 'https://user@store.example', ''])
      await expect(nativeEntrance(origin, {customerRequest})('/v2/customer/proof', '{}')).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    expect(customerRequest).not.toHaveBeenCalled();
  });

  test('the entrance of the build asks the native module, not fetch', async () => {
    const fetched = jest.spyOn(globalThis, 'fetch').mockImplementation(async () => { throw new Error('fetch must not be used'); });
    try {
      mockCustomerRequest.mockResolvedValue(answer(200, JSON.stringify({challenge: 'c1'.repeat(32), expiresAt: 1_764_000_060})));
      const entrance = configuredEntrance('https://store.example:8444');
      expect(entrance).toBeInstanceOf(CustomerEntrance);
      const issued = await entrance!.pickupChallenge(branch, sale);
      expect(issued.expiresAt).toBe(1_764_000_060);
      expect(mockCustomerRequest).toHaveBeenCalledWith('https://store.example:8444', '/v2/customer/challenge',
        JSON.stringify({purpose: 'pickup', branchRef: branch, saleRef: sale}));
      expect(fetched).not.toHaveBeenCalled();
    } finally {
      fetched.mockRestore();
    }
  });
});
