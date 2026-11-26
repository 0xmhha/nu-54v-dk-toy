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

test("the ready board reads as numbers only, and anything more is refused", async () => {
  const {readReadyBoard} = await import("../src/v2.ts");
  const board = {checkedAt: "2026-11-26T07:04:18Z", ready: [12, 11], making: [13], received: [15]};
  assert.deepEqual(readReadyBoard(board), board);
  for (const value of [undefined, {...board, amount: "1"}, {...board, ready: ["12"]}, {...board, ready: [0]}, {...board, checkedAt: "now"},
    {...board, making: Array.from({length: 41}, (_, i) => i + 1)}])
    assert.equal(readReadyBoard(value), undefined, String(JSON.stringify(value)).slice(0, 60));
});
