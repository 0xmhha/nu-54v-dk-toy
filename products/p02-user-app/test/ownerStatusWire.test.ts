import {readStatus} from '../src/wallet/ownerWire.ts';

// Owner status layout (firmware nu54_owner_status): version, state, root, profile, flags,
// generation (8), device hash (32), owner (32). Flags: bit 0 backup, bit 1 empty legacy bond store.
const device = 'ab'.repeat(32);
function status(flags: number): Uint8Array {
  const out = new Uint8Array(77);
  out.set([3, 1, 2, 1, flags]);
  out[12] = 2;
  out.set(Uint8Array.from({length: 32}, () => 0xab), 13);
  out.set(Uint8Array.from({length: 32}, () => 0xcd), 45);
  return out;
}

test.each([
  [0, false, false],
  [1, true, false],
  [2, false, true],
  [3, true, true],
])('status flags %i read as backup %s and empty bond store %s', (flags, backedUp, legacyBondsClear) => {
  expect(readStatus(status(flags), device)).toMatchObject({state: 'active', generation: 2n, backedUp, legacyBondsClear});
});

test.each([4, 0x80, 0xff])('unknown status flag bits %i are refused', flags => {
  expect(() => readStatus(status(flags), device)).toThrow('OWNER_BINDING');
});
