// The BLE central on the tablet: finds the device in payment mode and opens a fragment
// transport to it through the NusBle native module (payment-protocol.md 3).

import { GATT } from "@nu54/protocol";
import NusBle, { type FoundDevice } from "../specs/NativeNusBle.ts";
import { fromBase64, toBase64 } from "./base64.ts";
import type { FragmentTransport } from "./framing.ts";


export async function findDevice(timeoutMs = 4000): Promise<FoundDevice> {
  return NusBle.scan(GATT.service, timeoutMs);
}

/** Connects (no pairing, N27) and returns a transport; onDrop runs if the link drops. */
export async function openTransport(address: string, onDrop: (reason: string) => void): Promise<FragmentTransport> {
  const mtu = await NusBle.connect(address, GATT.service, GATT.rx, GATT.tx);
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
