// The device label QR (P02 design 1): nu54://bond?addr=<BLE address>&passkey=<6 digits>.
// opsctl rental provision prints it. The passkey is used for one bonding and not kept [N27].

export interface BondTarget {
  /** BLE address, upper-case AA:BB:CC:DD:EE:FF. */
  address: string;
  /** Six-digit passkey as printed (leading zeros kept). */
  passkey: string;
}

export function parseLabel(text: string): BondTarget {
  const m = /^nu54:\/\/bond\?(.*)$/.exec(text.trim());
  if (!m) throw new Error("not an NU54 device label");
  const params = new Map<string, string>();
  for (const pair of m[1].split("&")) {
    const i = pair.indexOf("=");
    if (i <= 0) throw new Error(`bad label field: ${pair}`);
    const key = pair.slice(0, i);
    if (params.has(key)) throw new Error(`repeated label field: ${key}`);
    params.set(key, decodeURIComponent(pair.slice(i + 1)));
  }
  const address = (params.get("addr") ?? "").toUpperCase();
  const passkey = params.get("passkey") ?? "";
  if (!/^([0-9A-F]{2}:){5}[0-9A-F]{2}$/.test(address)) throw new Error("label has no valid BLE address");
  if (!/^[0-9]{6}$/.test(passkey)) throw new Error("label passkey must be six digits");
  return { address, passkey };
}
