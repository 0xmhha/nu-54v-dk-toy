import OwnerWallet from '../specs/NativeOwnerWallet.ts';
import Ble from '../specs/NativeRenterBle.ts';
import {OwnerSession} from '../wallet/ownerSession.ts';
import {OWNER_GATT} from '../wallet/ownerWire.ts';
import {decodeWalletStorage} from '../wallet/profile.ts';
import type {BoardLink, PickupBoard} from './customerEntrance.ts';

// The board this phone is registered with, reached over Bluetooth for one account proof per pickup QR. The board ends
// a session about two minutes after it starts, so each QR opens a new one; the last board that answered is tried
// first, and a scan finds it again when it does not answer.

async function scan(seconds = 6): Promise<string[]> {
  const found = new Set<string>();
  const receipt = Ble.onScan(json => {
    try {
      const value = JSON.parse(json);
      if (typeof value.address === 'string' && value.address.length <= 64) found.add(value.address);
    } catch {}
  });
  try {
    await Ble.scanOwner(OWNER_GATT.service, seconds);
  } finally {
    receipt.remove();
  }
  return [...found];
}

let lastHandle: string | undefined;

export const ownerPickupBoard: PickupBoard = {
  async open(expected) {
    if (!OwnerWallet) throw new Error('BOARD_NOT_FOUND');
    const local = decodeWalletStorage(await OwnerWallet.loadProfile());
    if (!local.enrollment) throw new Error('BOARD_NOT_FOUND');
    const enrollment = local.enrollment, native = OwnerWallet;
    const tryHandle = async (handle: string): Promise<BoardLink | undefined> => {
      const session = new OwnerSession(Ble, native, enrollment, !!local.spendingBoard);
      try {
        await session.connect(handle);
        const status = await session.status();
        const {account} = await session.spendingStatus(status.generation);
        return {account, prove: challenge => session.proveAccount(account, challenge), close: () => session.close()};
      } catch {
        session.close();
        return undefined;
      }
    };
    // Another board's account is kept only to say which board is near; its session is closed at once.
    let other: string | undefined;
    const handles = lastHandle ? [lastHandle] : [];
    for (let round = 0; round < 2; round++) {
      for (const handle of handles) {
        const link = await tryHandle(handle);
        if (!link) continue;
        if (link.account === expected) { lastHandle = handle; return link; }
        other ??= link.account;
        link.close();
      }
      if (round === 0) handles.splice(0, handles.length, ...(await scan()).filter(handle => handle !== lastHandle));
    }
    // No board of this account answered; the first other one says which board is near.
    if (other) return {account: other, prove: async () => { throw new Error('ACCOUNT_PROOF'); }, close: () => {}};
    throw new Error('BOARD_NOT_FOUND');
  },
};
