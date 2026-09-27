import { protocolInfo } from "../src/protocolInfo";

test("kiosk sees the four payment outcomes from the shared protocol", () => {
  expect(protocolInfo.outcomes).toEqual(["approved", "refused", "failed", "Checking"]);
  expect(protocolInfo.deviceSignedTypes).toEqual(["PaymentAuthorization", "LimitChange"]);
});
