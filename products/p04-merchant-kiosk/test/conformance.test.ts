// Kiosk entry of the conformance harness (products/p10-platform/harness/README.md):
// PaymentAuthorization, LimitChange and MerchantOrder digests and signers, the MerchantOrder
// signature bytes, and the CBOR bodies and BLE fragments the kiosk sends and receives.
import {
  bytesToHex,
  decodeMessage,
  digest,
  encodeMessage,
  fragments,
  hexToBytes,
  Reassembler,
  type Message,
} from "@nu54/protocol";
import eip from "../../../docs/content/specifications/protocol/eip712-vectors.json";
import cbor from "../../../docs/content/specifications/protocol/cbor-vectors.json";
import frame from "../../../docs/content/specifications/protocol/frame-vectors.json";
import { checkDeviceAuthorization, limitChangeSigner, signMerchantOrder } from "../src/payment/signing";

// Foundry public test mnemonic, index 2 (merchant role). Test-only key.
const MERCHANT_TEST_KEY = hexToBytes("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a");
const domain = { chainId: eip.domain.chainId, verifyingContract: eip.domain.verifyingContract };
const vector = (id: string) => eip.vectors.find((v: { id: string }) => v.id === id)!;

describe("kiosk conformance", () => {
  test.each(["PA-01", "PA-02", "LC-01", "MO-01"])("%s digest", (id) => {
    const v = vector(id);
    expect(bytesToHex(digest(domain, v.primaryType as never, v.message as never))).toBe(v.digest);
  });

  test("MerchantOrder signature equals the vector byte for byte", () => {
    const v = vector("MO-01");
    expect(signMerchantOrder(domain, v.message as never, MERCHANT_TEST_KEY)).toBe(v.signature.toLowerCase());
  });

  test.each(["PA-01", "PA-02"])("%s device signature is accepted for the device and refused for another", (id) => {
    const v = vector(id);
    const { nonce, ...sent } = v.message as unknown as Record<string, string>;
    const returned = { signature: v.signature, nonce };
    expect(checkDeviceAuthorization(domain, sent as never, returned, v.signer)).toEqual({ ok: true, device: v.signer.toLowerCase() });
    expect(checkDeviceAuthorization(domain, sent as never, returned, vector("MA-01").signer).ok).toBe(false);
    const changed = { ...sent, amount: (BigInt(sent.amount) + 1n).toString() };
    expect(checkDeviceAuthorization(domain, changed as never, returned, v.signer)).toEqual({ ok: false, reason: "MERCHANT_FORGED" });
  });

  test("LimitChange signer is the device", () => {
    const v = vector("LC-01");
    expect(limitChangeSigner(domain, v.message as never, v.signature)).toBe(v.signer.toLowerCase());
  });

  test.each(cbor.vectors.map((v: { id: string }) => v.id))("%s CBOR body", (id) => {
    const v = cbor.vectors.find((x: { id: string }) => x.id === id)!;
    expect(bytesToHex(encodeMessage(v.message as unknown as Message), false)).toBe(v.cborHex);
    expect(decodeMessage(hexToBytes(v.cborHex)).type).toBe(v.message.type);
  });

  test("BLE fragments split and reassemble as the vectors", () => {
    const env = Object.fromEntries(cbor.vectors.map((v: { id: string; envelopeHex: string }) => [v.id, v.envelopeHex]));
    for (const v of frame.valid) {
      expect(fragments(hexToBytes(env[v.message]), v.sequence, v.attMtu).map((f) => bytesToHex(f, false))).toEqual(v.fragmentsHex);
      const r = new Reassembler();
      const results = v.fragmentsHex.map((h: string) => r.feed(hexToBytes(h)));
      expect(results[results.length - 1].status).toBe("done");
    }
    for (const v of frame.invalid) {
      const r = new Reassembler();
      expect(() => v.fragmentsHex.forEach((h: string) => r.feed(hexToBytes(h)))).toThrow(/BAD_FRAME/);
    }
  });
});
