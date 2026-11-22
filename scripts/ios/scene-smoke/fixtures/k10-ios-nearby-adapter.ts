// Match the synthetic payer registered by the signed W10 fixture.
import OwnerWallet from '../../../../products/p02-user-app/src/specs/NativeOwnerWallet.ts';
import nearby from './k10-nearby-adapter.ts';

export default {
  ...nearby,
  async configure(raw: string) {
    await nearby.configure(raw);
    if (JSON.parse(raw).signedReview !== true) return;
    if (!OwnerWallet) throw Error('LAB_OWNER_WALLET_REQUIRED');
    if (await OwnerWallet.loadProfile()) return;
    const board = 'a'.repeat(64);
    await OwnerWallet.saveProfile(JSON.stringify({
      version: 2,
      exercised: false,
      mode: 'personal',
      enrollment: {device: board},
      verified: {boardFingerprint: board, address: '0x' + '1'.repeat(40), network: '8283', generation: 1},
    }));
  },
};
