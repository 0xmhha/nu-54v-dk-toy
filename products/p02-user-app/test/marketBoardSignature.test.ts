import {hexToBytes} from '@nu54/protocol';
import {verifyMarketSigned} from '../src/wallet/cloud/client.ts';
import vector from './fixtures/p08-board-signed.json';

it('accepts the board host signature for the reviewed offer and rejects different terms or signer', () => {
  const raw = hexToBytes(vector.raw.slice(2));
  expect(verifyMarketSigned(raw, vector.transfer)).toBe(vector.raw);
  expect(() => verifyMarketSigned(raw, {...vector.transfer, nonce: '3'})).toThrow('MARKET_SIGNED_TERMS');
  const changedUser = {...vector.transfer.market, user: '0x0000000000000000000000000000000000000001'};
  expect(() => verifyMarketSigned(raw, {...vector.transfer, market: changedUser})).toThrow('MARKET_SIGNATURE');
  expect(() => verifyMarketSigned(raw.subarray(0, raw.length - 1), vector.transfer)).toThrow('MARKET_RLP');
});
