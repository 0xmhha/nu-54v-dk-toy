// Week-7 development setup: signs a fresh TimeAnchor whenever the kiosk app asks, so the device
// clock starts at the chain's time instead of whenever someone pushed an anchor file. The
// operator key stays on the Mac (opsctl, Keychain); the app reaches this server through
// `adb reverse` and gets an anchor for the device it is talking to.
//
// Usage (from the repository root, phone on USB):
//   node --experimental-strip-types products/p04-merchant-kiosk/scripts/anchor-server.ts
//   node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --attestation att.json --anchor-url http://127.0.0.1:8095/anchor
//
// Listens on 127.0.0.1 only; GET /anchor?device=0x<20 bytes> answers the TimeAnchor JSON.
// It also keeps `adb reverse` in place for itself and for Metro: a phone that drops off USB and
// comes back loses both, and the debug app then shows a blank screen until they are set again.

import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { resolve } from "node:path";
import { parseArgs } from "node:util";

const ROOT = resolve(import.meta.dirname, "../../..");
const { values: a } = parseArgs({
  options: {
    port: { type: "string", default: "8095" },
    opsctl: { type: "string", default: resolve(ROOT, "products/p05-operations-backoffice/bin/opsctl") },
    app: { type: "string", default: "com.nu54kiosk" },
    metro: { type: "string", default: "8081" }, // the debug app's JS bundler
  },
});
const port = Number(a.port);

const server = createServer((req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  const device = url.searchParams.get("device") ?? "";
  if (req.method !== "GET" || url.pathname !== "/anchor" || !/^0x[0-9a-fA-F]{40}$/.test(device)) {
    res.writeHead(400).end("GET /anchor?device=0x<20-byte address>\n");
    return;
  }
  try {
    const anchor = execFileSync(a.opsctl!, ["anchor", "sign", "--device", device], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
    const ts = JSON.parse(anchor).timestamp;
    console.error(`anchor for ${device.slice(0, 8)}…${device.slice(-4)} at ${ts}`);
    res.writeHead(200, { "content-type": "application/json" }).end(anchor);
  } catch (e) {
    console.error(`anchor failed: ${e instanceof Error ? e.message : String(e)}`);
    res.writeHead(500).end("opsctl anchor sign failed\n");
  }
});

/** The phone reaches the Mac's ports through USB; returns whether both forwards are in place. */
function reverse(): boolean {
  try {
    for (const p of [String(port), a.metro!]) execFileSync("adb", ["reverse", `tcp:${p}`, `tcp:${p}`], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

server.listen(port, "127.0.0.1", () => {
  console.error(`anchor server on 127.0.0.1:${port}; keeping adb reverse for ${port} and Metro ${a.metro}`);
  let up = reverse();
  setInterval(() => {
    const now = reverse();
    if (now !== up) console.error(now ? "phone back on USB: adb reverse set again" : "phone not on USB: waiting");
    up = now;
  }, 3000);
});
