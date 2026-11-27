import assert from "node:assert/strict";
import { localScheduleValue, scheduleCandidates } from "../src/localSchedule.ts";

assert.deepEqual(scheduleCandidates("2026-03-08T02:30", "America/New_York"), []);
const fold = scheduleCandidates("2026-11-01T01:30", "America/New_York");
assert.deepEqual(fold, ["2026-11-01T05:30:00.000Z", "2026-11-01T06:30:00.000Z"]);
assert.equal(localScheduleValue(fold[1], "America/New_York"), "2026-11-01T01:30");
assert.deepEqual(scheduleCandidates("2026-11-01T01:30", "Asia/Seoul"), ["2026-10-31T16:30:00.000Z"]);
