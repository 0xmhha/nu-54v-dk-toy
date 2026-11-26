import {test} from "node:test";
import assert from "node:assert/strict";
import {readOrderState} from "../src/v2.ts";

test("the store state sent with the menu reads as one of four, and anything else as unknown", () => {
  assert.deepEqual(readOrderState({mode: "open"}), {mode: "open"});
  assert.deepEqual(readOrderState({mode: "busy", busyMinutes: 25}), {mode: "busy", busyMinutes: 25});
  assert.deepEqual(readOrderState({mode: "paused", resumeAt: "11:20"}), {mode: "paused", resumeAt: "11:20"});
  assert.deepEqual(readOrderState({mode: "paused"}), {mode: "paused"});
  assert.deepEqual(readOrderState({mode: "closed"}), {mode: "closed"});
  assert.deepEqual(readOrderState({mode: "busy", busyMinutes: 10, taxFreeShop: "123-45-67890-001"}), {mode: "busy", busyMinutes: 10, taxFreeShop: "123-45-67890-001"});
  for (const value of [undefined, null, [], "open", {mode: "busy"}, {mode: "busy", busyMinutes: 90}, {mode: "busy", busyMinutes: 2.5},
    {mode: "paused", resumeAt: "25:00"}, {mode: "open", extra: 1}, {mode: "later"}, {mode: "open", taxFreeShop: "번호"}, {mode: "open", taxFreeShop: 5}])
    assert.equal(readOrderState(value), undefined, JSON.stringify(value));
});
