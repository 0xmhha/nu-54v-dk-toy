// Development provisioning of the kiosk app on a USB-connected Android device (N32).
//
// Writes provision.json (settings, the operator-signed attestation, and the kiosk's gas key and
// merchant signing key) or anchor.json (a fresh TimeAnchor) into the app's private files
// directory with `adb shell run-as`, which works only for a debuggable build. The app takes the
// file on its next start or payment, wraps the keys with its Android Keystore key and deletes it.
// The keys cross the USB cable in the clear: development builds only, never a release.
//
// Usage (from the repository root):
//   O=products/p05-operations-backoffice/bin/opsctl
//   $O attestation issue --merchant <kiosk> --payout <payout> --name "NU54 Test Cafe" > att.json
//   node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --attestation att.json
//   $O anchor sign --device <device> > anchor.json        # right before a run
//   node --experimental-strip-types products/p04-merchant-kiosk/scripts/provision-dev.ts --anchor anchor.json
// Keys: nu54-kiosk (password from the Keychain).

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { bytesToHex } from "@nu54/protocol";
import { openKeystore } from "../../../packages/device-sim/src/keystore.ts";

const ROOT = resolve(import.meta.dirname, "../../..");
const { values: a } = parseArgs({
  options: {
    attestation: { type: "string" },
    anchor: { type: "string" },
    app: { type: "string", default: "com.nu54kiosk" },
    rpc: { type: "string", default: "https://api.test.stablenet.network/" },
    indexer: { type: "string" }, // P07 base URL reachable from the tablet, for the receipt screen
    "anchor-url": { type: "string" }, // scripts/anchor-server.ts through adb reverse: fresh anchors on demand
    deployment: { type: "string", default: join(ROOT, "products/p06-stablenet-contracts/deployments/8283.json") },
    keystores: { type: "string", default: join(homedir(), ".nu54", "keystores") },
  },
});
if (!a.attestation && !a.anchor) throw new Error("--attestation (provision) or --anchor (fresh anchor) is required");

/** Writes text into the app's private files directory through run-as (stdin, no temp file). */
function push(name: string, text: string): void {
  execFileSync("adb", ["shell", `run-as ${a.app} sh -c 'mkdir -p files && cat > files/${name}'`], { input: text, stdio: ["pipe", "inherit", "inherit"] });
  console.error(`wrote ${name} into ${a.app}; the app takes it on its next start or payment`);
}

if (a.attestation) {
  const dep = JSON.parse(readFileSync(a.deployment!, "utf8"));
  const register = JSON.parse(readFileSync(join(ROOT, "docs/content/planning/design-freeze-checkpoint-02.json"), "utf8"));
  const minGas = register.parameters.kioskMinGasBalance;
  if (minGas.unit !== "WKRC") throw new Error(`unexpected kioskMinGasBalance unit ${minGas.unit}`);
  const key = bytesToHex(openKeystore(join(a.keystores!, "nu54-kiosk"), "keychain:nu54-kiosk"));
  push("provision.json", JSON.stringify({
    config: {
      rpc: a.rpc,
      chainId: dep.chainId,
      settlement: dep.contracts.PaymentSettlement.address,
      token: dep.contracts.TestUSDC.address,
      tokenSymbol: "tUSDC",
      tokenDecimals: 6,
      fromBlock: String(dep.contracts.PaymentSettlement.block),
      minGasBalanceWei: (BigInt(minGas.value) * 10n ** 18n).toString(),
      attestation: JSON.parse(readFileSync(a.attestation, "utf8")),
      ...(a.indexer ? { indexerUrl: a.indexer } : {}),
      ...(a["anchor-url"] ? { anchorUrl: a["anchor-url"] } : {}),
    },
    // One key pays gas and signs orders today (the kiosk role account); the app keeps them apart.
    keys: { gas: key, merchant: key },
  }));
}
if (a.anchor) push("anchor.json", readFileSync(a.anchor, "utf8"));
