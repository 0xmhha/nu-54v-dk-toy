// The BLE central on the tablet: finds the device in payment mode and opens a fragment
// transport to it through the NusBle native module (payment-protocol.md 3).

import { GATT } from "@nu54/protocol";
import NusBle, { type FoundDevice } from "../specs/NativeNusBle.ts";
import { fromBase64, toBase64 } from "@nu54/protocol";
import type { FragmentTransport } from "./framing.ts";


export async function findDevice(timeoutMs = 4000): Promise<FoundDevice> {
  return NusBle.scan(GATT.service, timeoutMs);
}

/**
 * A first BLE connection sometimes fails to establish (HCI 0x3e, GATT status 133) and works when
 * tried again; the kiosk tries up to `attempts` times before it reports the device unreachable.
 */
export async function connectWithRetry<T>(connect: () => Promise<T>, attempts = 3, pauseMs = 300): Promise<T> {
  let last: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await connect();
    } catch (e) {
      last = e;
      if (i + 1 < attempts) await new Promise<void>((r) => setTimeout(() => r(), pauseMs));
    }
  }
  throw last;
}

/** Connects (no pairing, N27) and returns a transport; onDrop runs if the link drops. */
export async function openTransport(address: string, onDrop: (reason: string) => void): Promise<FragmentTransport> {
  const mtu = await connectWithRetry(() => NusBle.connect(address, GATT.service, GATT.rx, GATT.tx));
  const dropped = NusBle.onDisconnect(onDrop);
  return {
    mtu,
    write: (fragment) => NusBle.writeFragment(toBase64(fragment)),
    onFragment(handler) {
      const sub = NusBle.onFragment((b64) => handler(fromBase64(b64)));
      return () => sub.remove();
    },
    async close() {
      dropped.remove();
      await NusBle.disconnect();
    },
  };
}
