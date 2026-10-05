// The BLE connect retry (src/ble/central.ts): a first connection that fails to establish is retried.
import { connectWithRetry } from "../src/ble/central.ts";

jest.mock("../src/specs/NativeNusBle.ts", () => ({ __esModule: true, default: {} }));

test("a connection that fails twice and then works is returned", async () => {
  let calls = 0;
  const v = await connectWithRetry(async () => {
    if (++calls < 3) throw new Error("CONNECT_FAILED status 133");
    return 247;
  }, 3, 1);
  expect([v, calls]).toEqual([247, 3]);
});

test("after the last attempt the last error is reported", async () => {
  let calls = 0;
  await expect(connectWithRetry(async () => { calls++; throw new Error(`fail ${calls}`); }, 3, 1)).rejects.toThrow("fail 3");
  expect(calls).toBe(3);
});
