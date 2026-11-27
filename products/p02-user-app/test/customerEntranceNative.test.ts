import {configuredEntrance, CustomerEntrance, entranceAnchor, forgetEntranceWaits, longestWait, nativeEntrance} from '../src/merchant/customerEntrance';

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
  beforeEach(() => { mockCustomerRequest.mockReset(); forgetEntranceWaits(); });

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

  test('the next page of the store list and a cart taken back go out as they are, the cart with its session', async () => {
    const customerRequest = jest.fn(async (_anchor: string, _path: string, _session: string, _body: string) => answer(200, '{}'));
    const post = nativeEntrance(anchor, {customerRequest});
    await post(`/v2/customer/stores?after=${branch}`, '');
    await post('/v2/customer/checkout/cancel', JSON.stringify({saleRef: sale}), '5e'.repeat(32));
    expect(customerRequest.mock.calls).toEqual([[sent, `/v2/customer/stores?after=${branch}`, '', ''],
      [sent, '/v2/customer/checkout/cancel', '5e'.repeat(32), JSON.stringify({saleRef: sale})]]);
  });

  describe('the wait a server asks for', () => {
    const busy = (retryAfter?: unknown, status = 429, more = {}) =>
      JSON.stringify({status, body: JSON.stringify({requestId: '', requestDigest: '', status: 'error', error: 'BUSY', ...more}), ...(retryAfter === undefined ? {} : {retryAfter})});
    beforeEach(() => { jest.useFakeTimers({now: 1_764_000_000_000}); });
    afterEach(() => { jest.useRealTimers(); });

    test('comes back with the refusal, in seconds, and never longer than the phone waits', async () => {
      const replies = [busy(7), busy(100_000), busy(0), busy('7'), busy(1.5), busy()];
      const customerRequest = jest.fn(async () => replies.shift()!);
      const waits: unknown[] = [];
      for (let n = 0; n < 6; n++) {
        forgetEntranceWaits();
        waits.push((await nativeEntrance(anchor, {customerRequest})('/v2/customer/proof', '{}')).retryAfter);
      }
      expect(waits).toEqual([7, longestWait, undefined, undefined, undefined, undefined]);
    });

    test.each([[429, 5], [503, 1]])('a %i for the rate holds the next request of its kind for the time asked, and the phone sends nothing until then', async (status, wait) => {
      const customerRequest = jest.fn(async (_anchor: string, _path: string, _session: string, _body: string) => busy(wait, status));
      const post = nativeEntrance(anchor, {customerRequest});
      expect(await post('/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32))).toMatchObject({status, retryAfter: wait});
      // Another entrance of the same build, as another screen makes one, waits out the same answer.
      const other = nativeEntrance(anchor, {customerRequest});
      await expect(other('/v2/customer/commerce', '{"command":"y"}', '5e'.repeat(32))).rejects.toMatchObject({code: 'BUSY', retryAfter: wait});
      await expect(post('/v2/customer/checkout/cancel', '{"saleRef":"x"}', '5e'.repeat(32))).rejects.toMatchObject({code: 'BUSY'});
      jest.advanceTimersByTime(wait * 1000 - 1);
      await expect(post('/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32))).rejects.toMatchObject({code: 'BUSY', retryAfter: 1});
      expect(customerRequest).toHaveBeenCalledTimes(1);
      jest.advanceTimersByTime(1);
      customerRequest.mockResolvedValueOnce(answer(200, '{}'));
      expect(await post('/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32))).toEqual({status: 200, body: {}});
      expect(customerRequest).toHaveBeenCalledTimes(2);
    });

    test('a wait is for the kind of request the server counts together, and for that server only', async () => {
      const customerRequest = jest.fn(async (_anchor: string, path: string, _session: string, body: string) =>
        path === '/v2/customer/challenge' && body.includes('checkout') ? busy(10) : answer(200, '{}'));
      const post = nativeEntrance(anchor, {customerRequest});
      await post('/v2/customer/challenge', JSON.stringify({purpose: 'checkout', branchRef: branch}));
      // A new session waits; a pickup QR at the counter, a proof, the menu, a photo and the session's requests do not.
      await expect(post('/v2/customer/challenge', JSON.stringify({purpose: 'order', branchRef: branch, saleRef: sale}))).rejects.toMatchObject({code: 'BUSY'});
      for (const [path, body, session] of [['/v2/customer/challenge', JSON.stringify({purpose: 'pickup', branchRef: branch, saleRef: sale}), ''], ['/v2/customer/proof', '{}', ''],
        ['/v2/customer/stores', '', ''], [`/v2/customer/stores/${branch}/menu/0`, '', ''], [`/v2/customer/stores/${branch}/photos/${'a'.repeat(64)}/0`, '', ''],
        ['/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32)]] as const)
        expect((await post(path, body, session)).status).toBe(200);
      const elsewhere = nativeEntrance({...anchor, origin: 'https://other.example:8444'}, {customerRequest: async () => answer(200, '{}')});
      expect((await elsewhere('/v2/customer/challenge', JSON.stringify({purpose: 'checkout', branchRef: branch}))).status).toBe(200);
    });

    test('a refusal that names a reason, and any other answer, holds nothing back', async () => {
      const replies = [busy(10, 429, {reason: 'hold_limit'}), busy(3, 503, {reason: 'balance_unavailable'}), JSON.stringify({status: 404, body: 'Not Found', retryAfter: 9}),
        JSON.stringify({status: 503, body: JSON.stringify({status: 'error', error: 'UNKNOWN_RESULT'}), retryAfter: 9}), busy(undefined), answer(200, '{}')];
      const customerRequest = jest.fn(async () => replies.shift()!);
      const post = nativeEntrance(anchor, {customerRequest});
      for (let n = 0; n < 6; n++) await post('/v2/customer/commerce', '{"command":"x"}', '5e'.repeat(32));
      expect(customerRequest).toHaveBeenCalledTimes(6);
    });
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
