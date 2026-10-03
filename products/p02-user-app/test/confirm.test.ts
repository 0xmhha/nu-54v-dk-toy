// Confirmation screen strings, the label QR and the bonded-link rules (P02 design 6).
import eip from "../../../docs/content/specifications/protocol/eip712-vectors.json";
import sessionVectors from "../../../docs/content/specifications/protocol/session-vectors.json";
import { bytesToHex, encodeMessage, FrameWriter, hexToBytes, Reassembler, type Message } from "@nu54/protocol";
import { checksumAddress, confirmView, limitView, truncatedAmount } from "../src/confirm/display.ts";
import { parseLabel } from "../src/qr.ts";
import { ConfirmLink, CONFIRM_TIMEOUT_MS, type Screen } from "../src/link/confirmLink.ts";

const PA01 = eip.vectors.find((v) => v.id === "PA-01") as unknown as {
  signer: string;
  message: { merchant: string; payout: string; token: string; amount: string; orderId: string };
};
const TOKEN = PA01.message.token.toLowerCase();
const TABLE = { [TOKEN]: { symbol: "USDC", decimals: 6 } };

test("EIP-55 checksum matches the vectors' checksummed addresses", () => {
  expect(checksumAddress(PA01.message.merchant.toLowerCase())).toBe(PA01.message.merchant);
  expect(checksumAddress(PA01.signer.toLowerCase())).toBe(PA01.signer);
  expect(() => checksumAddress("0x1234")).toThrow();
});

test("amounts are truncated to two decimals, never rounded (N31)", () => {
  expect(truncatedAmount(5_009_999n, 6)).toBe("5.00");
  expect(truncatedAmount(4_500_000n, 6)).toBe("4.50");
  expect(truncatedAmount(999n, 6)).toBe("0.00");
  expect(truncatedAmount(123_456_789_012_345_678_901n, 18)).toBe("123.45");
});

test("the screen strings come from PA-01's fields", () => {
  const show = { merchantName: "Cafe Test 01", orderId: PA01.message.orderId, token: PA01.message.token, payout: PA01.message.payout, amount: PA01.message.amount };
  expect(confirmView(show, TABLE)).toEqual({
    merchantName: "Cafe Test 01", amount: "4.50 USDC", token: "USDC", known: true,
    payout: checksumAddress(PA01.message.payout), orderShort: "0x010101…0101",
  });
  // An unknown token: base units and the address, no symbol.
  const unknown = confirmView(show, {});
  expect([unknown.amount, unknown.known, unknown.token]).toEqual(["4500000", false, checksumAddress(PA01.message.token)]);
});

test("label QR", () => {
  expect(parseLabel("nu54://bond?addr=d4:3f:2a:10:00:9b&passkey=042195")).toEqual({ address: "D4:3F:2A:10:00:9B", passkey: "042195" });
  for (const bad of ["nu54://bond?addr=D4:3F:2A:10:00:9B&passkey=42195", "nu54://bond?passkey=042195", "https://x?addr=D4:3F:2A:10:00:9B&passkey=042195",
    "nu54://bond?addr=D4:3F:2A:10:00:9B&passkey=042195&passkey=111111"]) {
    expect(() => parseLabel(bad)).toThrow();
  }
});

// ---------------------------------------------------------------- the link

function phone(bonded = true) {
  let handler: ((f: Uint8Array) => void) | null = null;
  const screens: Screen[] = [];
  const onFragment = (h: (f: Uint8Array) => void) => {
    handler = h;
    return () => (handler = null);
  };
  const link = new ConfirmLink({ bonded: () => bonded, onFragment }, (s) => screens.push(s), TABLE);
  const writer = new FrameWriter(23);
  const deliverBody = (body: Uint8Array) => writer.write(body).forEach((f) => handler?.(f));
  const deliver = (m: Message) => deliverBody(encodeMessage(m));
  return { link, screens, deliver, deliverBody };
}

/** A confirm.show for PA-01's payment. */
function deviceShow(): Message {
  return {
    v: 1, type: "confirm.show", sessionId: "0102030405060708", merchantName: "Cafe Test 01",
    orderId: PA01.message.orderId, token: PA01.message.token, payout: PA01.message.payout, amount: PA01.message.amount,
  } as Message;
}

test("a bonded link shows confirm.show, then the outcome of that order", () => {
  const p = phone();
  p.deliver(deviceShow());
  expect(p.link.current()).toMatchObject({ kind: "confirming", view: { amount: "4.50 USDC" } });
  p.deliver({ v: 1, type: "payment.outcome", sessionId: "0102030405060708", orderId: PA01.message.orderId, outcome: "approved" } as Message);
  expect(p.link.current()).toMatchObject({ kind: "result", outcome: "approved", view: { merchantName: "Cafe Test 01" } });
});

test("an unbonded link cannot drive the screen (P02-FR-02)", () => {
  const p = phone(false);
  p.deliver(deviceShow());
  expect(p.link.current()).toEqual({ kind: "waiting" });
  expect(p.link.dropped).toBe(1);
});

test("a confirmation without an outcome goes back to waiting", () => {
  jest.useFakeTimers();
  const p = phone();
  p.deliver(deviceShow());
  jest.advanceTimersByTime(CONFIRM_TIMEOUT_MS + 1);
  expect(p.link.current()).toEqual({ kind: "waiting" });
  jest.useRealTimers();
});

test("the software device's confirm.show bytes decode to the same screen", () => {
  // The phone and the device share the protocol code: a confirm.show from the device's payment
  // flow (session vector SV-01) produces the PA-01 screen.
  const step = sessionVectors.scenarios[0].steps.find((st) => (st.phone?.length ?? 0) > 0) as { phone: string[] };
  const p = phone();
  p.deliverBody(hexToBytes(step.phone[0]));
  expect(p.link.current()).toMatchObject({ kind: "confirming", view: { merchantName: "Cafe Test 01", payout: checksumAddress(PA01.message.payout) } });
});

test("the device's phone messages from SV-01 end on the approved result for that order", () => {
  // SV-01's second payment: confirm.show at prepare, then the forwarded payment.outcome.
  const steps = sessionVectors.scenarios[0].steps.filter((st) => (st.phone?.length ?? 0) > 0) as { phone: string[] }[];
  const p = phone();
  for (const st of steps.slice(-2)) p.deliverBody(hexToBytes(st.phone[0]));
  expect(p.link.current()).toMatchObject({ kind: "result", outcome: "approved", view: { merchantName: "Cafe Test 01" } });
});

test("confirm.limit from SV-18 shows the limits; 0 reads as the cap", () => {
  const step = sessionVectors.scenarios.find((sc) => sc.id === "SV-18")!.steps.find((st) => (st.phone?.length ?? 0) > 0) as { phone: string[] };
  const p = phone();
  p.deliverBody(hexToBytes(step.phone[0]));
  expect(p.link.current()).toEqual({ kind: "limit", view: { perPayment: "20.00 tUSDC", daily: "100.00 tUSDC", expiry: 1790000060 } });
});

test("a zero limit means the cap", () => {
  expect(limitView({ perPaymentLimit: "0", dailyLimit: "5009999", expiry: "1" })).toEqual({ perPayment: "상한 그대로", daily: "5.00 tUSDC", expiry: 1 });
});

// ---------------------------------------------------------------- payment mode (P02-FR-08)

test("payment mode: the phone app writes the SV-32 bytes and reads the device's answer", async () => {
  const sv32 = (sessionVectors.scenarios as { id: string; steps: { send: string; expect: string[] }[] }[]).find((s) => s.id === "SV-32")!;
  const on120 = sv32.steps[3], on0 = sv32.steps[4], off = sv32.steps[6];
  let handler: ((f: Uint8Array) => void) | null = null;
  const written: Uint8Array[] = [];
  const link = new ConfirmLink({
    bonded: () => true,
    onFragment: (h) => { handler = h; return () => (handler = null); },
    write: async (f) => { written.push(f); },
    mtu: 185,
  }, () => {}, TABLE);
  const answers: unknown[] = [];
  link.onPaymentMode = (m) => answers.push(m);
  const rx = new Reassembler();
  const sent = () => {
    let body: Uint8Array | null = null;
    for (const f of written.splice(0)) {
      const r = rx.feed(f);
      if (r.status === "done") body = r.body;
    }
    return body ? bytesToHex(body, false) : null;
  };
  const writer = new FrameWriter(185);
  const answer = (hex: string) => writer.write(hexToBytes(hex)).forEach((f) => handler?.(f));

  await link.setPaymentMode(true);
  expect(sent()).toBe(on120.send);
  answer(on120.expect[0]);
  await link.setPaymentMode(false);
  expect(sent()).toBe(off.send);
  answer(on0.expect[0]); // a refusal, as the device sends it for on with 0 s
  expect(answers).toEqual([{ on: true, accepted: true }, { on: true, accepted: false, reason: "NOT_PERMITTED" }]);
});

test("payment mode needs a link that can write and is bonded", async () => {
  const p = phone();
  await expect(p.link.setPaymentMode(true)).rejects.toThrow("cannot write");
  const unbonded = new ConfirmLink({ bonded: () => false, onFragment: () => () => {}, write: async () => {}, mtu: 23 }, () => {});
  await expect(unbonded.setPaymentMode(true)).rejects.toThrow("bonded");
});
