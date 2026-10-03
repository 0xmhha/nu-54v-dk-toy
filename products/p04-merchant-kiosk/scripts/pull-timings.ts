// Reads the kiosk's order records off a USB-connected tablet (debuggable build, adb run-as) and
// writes the W12-04 timing CSV: the last 20 signed payments, request delivered -> final result.
//
// Usage (from the repository root):
//   node --experimental-strip-types products/p04-merchant-kiosk/scripts/pull-timings.ts > runs.csv
//   python3 products/p10-platform/acceptance/w12.py timings W12-04 runs.csv

import { execFileSync } from "node:child_process";
import { parseArgs } from "node:util";
import { ordersFromSharedPrefs, timingCsv, timingRows } from "../src/kiosk/timings.ts";

const { values: a } = parseArgs({ options: { app: { type: "string", default: "com.nu54kiosk" }, count: { type: "string", default: "20" } } });
const xml = execFileSync("adb", ["shell", `run-as ${a.app} cat shared_prefs/nu54-settings.xml`], { encoding: "utf8" });
const rows = timingRows(ordersFromSharedPrefs(xml), Number(a.count));
process.stdout.write(timingCsv(rows));
const slow = rows.filter((r) => r.ms > 10_000 || r.outcome !== "approved");
console.error(`${rows.length} payments, ${slow.length} not approved within 10 s`);
