import { execFileSync } from "node:child_process";
import { X509Certificate } from "node:crypto";
import { mkdirSync, readFileSync, realpathSync, writeFileSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { secp256k1 } from "@noble/curves/secp256k1";
import { quoteDigest, quoteV2Layout } from "../../../packages/protocol/ts/src/quote-v2.ts";
import { addressOfPublicKey, signDigest } from "../../../packages/protocol/ts/src/signature.ts";
import { v2SpkiSha256 } from "../../../packages/protocol/ts/src/transport-v2-node.ts";

const location = process.argv[2];
if (!location || !isAbsolute(location)) throw new Error("Use an absolute new directory outside the source checkout");
const repo = realpathSync(fileURLToPath(new URL("../../../", import.meta.url)));
const folder = join(realpathSync(dirname(location)), basename(location));
const relation = relative(repo, folder);
if (!relation.startsWith(".." + sep)) throw new Error("Pilot keys must live outside the source checkout");
mkdirSync(folder, { mode: 0o700 });
process.umask(0o077);
const out = (name: string) => join(folder, name);
const openssl = (...args: string[]) => execFileSync(process.env.OPENSSL ?? "openssl", args, { stdio: "pipe" });
const root = out("ca.pem"), rootKey = out("ca.key");
openssl("req", "-new", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
  "-days", "1", "-subj", "/CN=NU54 Disposable Pilot Root", "-keyout", rootKey, "-out", root);
let serial = 1;
function certificate(role: string) {
  const key = out(`${role}.key`), csr = out(`${role}.csr`), cert = out(`${role}.pem`), ext = out(`${role}.ext`);
  openssl("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
    "-subj", `/CN=Disposable ${role}`, "-keyout", key, "-out", csr);
  writeFileSync(ext, `subjectAltName=URI:urn:nu54:role:${role}\nbasicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=clientAuth,serverAuth\n`);
  openssl("x509", "-req", "-in", csr, "-CA", root, "-CAkey", rootKey, "-set_serial", String(serial++),
    "-days", "1", "-extfile", ext, "-out", cert);
  return v2SpkiSha256(new X509Certificate(readFileSync(cert)));
}
certificate("board");
certificate("owner");
certificate("terminal");
const vector = readFileSync(new URL("../../../packages/protocol/v2/testdata/quote.vectors", import.meta.url), "utf8")
  .split("\n").find((line) => line.startsWith("phone-private|"))!.split("|");
const quote = Buffer.from(vector[2], "hex");
const fixtureTime = BigInt(Math.floor(Date.now() / 1000));
const payer = secp256k1.utils.randomPrivateKey();
const merchant = secp256k1.utils.randomPrivateKey();
const address = (key: Uint8Array) => Buffer.from(addressOfPublicKey(secp256k1.getPublicKey(key, false)), "hex");
quote.set(address(payer), quoteV2Layout.payerOffset + 12);
quote.writeBigUInt64BE(1n, quoteV2Layout.payerGenerationOffset + 24);
quote.writeBigUInt64BE(fixtureTime - 1n, quoteV2Layout.issuedAtOffset + 24);
quote.writeBigUInt64BE(fixtureTime + 120n, quoteV2Layout.expiresAtOffset + 24);
const signed = Buffer.concat([quote, Buffer.from(signDigest(quoteDigest(quote), merchant))]);
writeFileSync(out("signed-quote.bin"), signed);
process.stdout.write(out("signed-quote.bin") + "\n");
