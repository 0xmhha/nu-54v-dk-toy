import {configuredEntrance, CustomerEntrance, entranceAnchor, nativeEntrance} from '../src/merchant/customerEntrance';

const mockCustomerRequest = jest.fn<Promise<string>, [string, string, string, string]>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  customerRequest: (anchor: string, path: string, session: string, body: string) => mockCustomerRequest(anchor, path, session, body),
  loadProfile: () => { throw new Error('the entrance must not read an enrollment profile'); },
}}));

const answer = (status: unknown, body: unknown) => JSON.stringify({status, body});
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const ca = 'MIIB' + 'A'.repeat(64), pin = 'ab'.repeat(32);
const anchor = {origin: 'https://store.example:8444', ca, pin};
const sent = JSON.stringify({origin: 'https://store.example:8444', ca, pin});

describe('customer entrance over the native commerce module', () => {
  beforeEach(() => mockCustomerRequest.mockReset());

  test('sends the build anchor with the canonical https origin and hands back the status and the parsed body', async () => {
    const customerRequest = jest.fn(async (_anchor: string, _path: string, _session: string, _body: string) => answer(200, '{"ok":true}'));
    const post = nativeEntrance({...anchor, origin: 'https://Store.Example:8444/'}, {customerRequest});
    expect(await post('/v2/customer/challenge', '{"a":1}')).toEqual({status: 200, body: {ok: true}});
    expect(customerRequest.mock.calls).toEqual([[sent, '/v2/customer/challenge', '', '{"a":1}']]);
  });

  test('a session token goes with the request that names it, and a read has no body', async () => {
    const customerRequest = jest.fn(async (_anchor: string, _path: string, _session: string, _body: string) => answer(200, '{}'));
    const post = nativeEntrance(anchor, {customerRequest});
    await post('/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32));
    await post('/v2/customer/stores', '');
    expect(customerRequest.mock.calls).toEqual([[sent, '/v2/customer/commerce', '5e'.repeat(32), '{"command":"x"}'],
      [sent, '/v2/customer/stores', '', '']]);
  });

  test('a refusal keeps its status, and a body that is not JSON reads as null', async () => {
    const replies = [answer(429, '{"status":"error","error":"BUSY"}'), answer(404, '404 page not found\n'), answer(503, '')];
    const post = nativeEntrance(anchor, {customerRequest: async () => replies.shift()!});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 429, body: {status: 'error', error: 'BUSY'}});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 404, body: null});
    expect(await post('/v2/customer/proof', '{}')).toEqual({status: 503, body: null});
  });

  test('no native method, a native failure and a malformed reply are all unavailable', async () => {
    const failing = {customerRequest: async () => { throw Object.assign(new Error('x'), {code: 'COMMERCE_UNAVAILABLE'}); }};
    const cases = [null, {}, failing, {customerRequest: async () => 'not json'}, {customerRequest: async () => 'null'},
      {customerRequest: async () => answer('200', '{}')}, {customerRequest: async () => answer(200, {ok: true})}];
    for (const http of cases)
      await expect(nativeEntrance(anchor, http)('/v2/customer/proof', '{}')).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
  });

  test('the current key and the next one go to the native module as a list of two', async () => {
    const next = 'cd'.repeat(32);
    const customerRequest = jest.fn(async (_anchor: string, _path: string, _session: string, _body: string) => answer(200, '{}'));
    await nativeEntrance({...anchor, pin: [pin, next]}, {customerRequest})('/v2/customer/stores', '');
    expect(customerRequest.mock.calls[0][0]).toBe(JSON.stringify({origin: anchor.origin, ca, pin: [pin, next]}));
    for (const pins of [[], [pin, pin], [pin, next, 'ef'.repeat(32)], [pin, '0'.repeat(64)], [pin, 5]])
      expect(entranceAnchor({...anchor, pin: pins as string[]})).toBeUndefined();
  });

  test('an anchor that is missing, not https, or without a CA or a pin never reaches the native module', async () => {
    const customerRequest = jest.fn(async () => answer(200, '{}'));
    const broken = [undefined, {...anchor, origin: 'http://store.example'}, {...anchor, origin: 'https://store.example/path'},
      {...anchor, origin: 'https://user@store.example'}, {...anchor, origin: ''}, {...anchor, ca: ''}, {...anchor, ca: 'not base64!'},
      {...anchor, pin: ''}, {...anchor, pin: 'AB'.repeat(32)}, {...anchor, pin: '0'.repeat(64)}];
    for (const value of broken) {
      expect(entranceAnchor(value)).toBeUndefined();
      await expect(nativeEntrance(value, {customerRequest})('/v2/customer/proof', '{}')).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    }
    expect(customerRequest).not.toHaveBeenCalled();
  });

  test('the entrance of the build asks the native module with its anchor: no fetch, no enrollment profile', async () => {
    const fetched = jest.spyOn(globalThis, 'fetch').mockImplementation(async () => { throw new Error('fetch must not be used'); });
    try {
      mockCustomerRequest.mockResolvedValue(answer(200, JSON.stringify({challenge: 'c1'.repeat(32), expiresAt: 1_764_000_060})));
      const entrance = configuredEntrance(anchor);
      expect(entrance).toBeInstanceOf(CustomerEntrance);
      const issued = await entrance!.pickupChallenge(branch, sale);
      expect(issued.expiresAt).toBe(1_764_000_060);
      expect(mockCustomerRequest).toHaveBeenCalledWith(sent, '/v2/customer/challenge', '',
        JSON.stringify({purpose: 'pickup', branchRef: branch, saleRef: sale}));
      expect(fetched).not.toHaveBeenCalled();
      // The shipped build binds no anchor yet, so it has no entrance.
      expect(configuredEntrance()).toBeUndefined();
    } finally {
      fetched.mockRestore();
    }
  });
});
