import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { constants, X509Certificate } from "node:crypto";
import { once } from "node:events";
import { mkdtempSync, readFileSync, rmSync, renameSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { connect, type ConnectionOptions } from "node:tls";
import { secp256k1 } from "@noble/curves/secp256k1";
import { keccak_256 } from "@noble/hashes/sha3";
import { quoteDigest, quoteV2Layout, verifyQuote } from "../src/quote-v2.ts";
import { fromBase64, toBase64 } from "../src/base64.ts";
import { bytesToHex } from "../src/bytes.ts";
import { rentalReviewPrepare, verifyRentalPaymentReview, verifyRentalPaymentDecision } from "../src/rental-payment-review.ts";
import { addressOfPrivateKey, recoverSigner, signDigest } from "../src/signature.ts";
import { bindV2NodeTls, v2SpkiSha256 } from "../src/transport-v2-node.ts";
import {
  encodeApprovalProgressOffer, encodeV2StreamFrame, V2_ALPN, V2_EXPORTER_LABEL, V2Journal, V2StreamDecoder,
  type V2Link, type V2Profile, type V2Request, type V2Role,
} from "../src/transport-v2.ts";

const peerBinary = process.env.NU54_V2_TLS_TEST_BIN;
assert(peerBinary, "build the independent C TLS endpoint first");
const vectorFile = new URL("../../v2/testdata/quote.vectors", import.meta.url);
const vector = readFileSync(vectorFile, "utf8").split("\n").find((line) => line.startsWith("phone-private|"))!.split("|");
const quote = Buffer.from(vector[1], "hex");
const policy = {
  expected: Buffer.from(vector[2], "hex"), merchantSigner: Buffer.from(vector[4], "hex"),
  now: BigInt(vector[5]), consumed: false,
};
assert.equal(verifyQuote(quote, policy), quoteV2Layout.statusOk);
const quoteHash = Buffer.from(quoteDigest(quote.subarray(0, quoteV2Layout.unsignedSize))).toString("hex");
assert.equal(quoteHash, vector[7]);

function seconds(): bigint {
  return BigInt(Math.floor(Date.now() / 1000));
}

type Certificate = { key: string; cert: string; fingerprint: Buffer };

test("real C/OpenSSL and TS/Node TLS endpoints carry and reject version-two frames", { timeout: 25000 }, async (t) => {
  const temp = mkdtempSync(join(tmpdir(), "nu54-mtls-v2-"));
  t.after(() => rmSync(temp, { recursive: true, force: true }));
  const file = (name: string) => join(temp, name);
  const openssl = (...args: string[]) => execFileSync(process.env.OPENSSL ?? "openssl", args, { stdio: "pipe" });
  const ca = file("ca.pem");
  const caKey = file("ca.key");
  openssl("req", "-new", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
    "-days", "2", "-subj", "/CN=NU54 Interop Test Root", "-keyout", caKey, "-out", ca);
  let serial = 1;
  function certificate(name: string, role: V2Role): Certificate {
    const key = file(`${name}.key`), csr = file(`${name}.csr`), cert = file(`${name}.pem`), ext = file(`${name}.ext`);
    openssl("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
      "-subj", `/CN=${name}`, "-keyout", key, "-out", csr);
    writeFileSync(ext, `subjectAltName=URI:urn:nu54:role:${role}\nbasicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=clientAuth,serverAuth\n`);
    openssl("x509", "-req", "-in", csr, "-CA", ca, "-CAkey", caKey, "-set_serial", String(serial++),
      "-days", "2", "-extfile", ext, "-out", cert);
    return { key, cert, fingerprint: v2SpkiSha256(new X509Certificate(readFileSync(cert))) };
  }
  const owner = certificate("owner", "owner");
  const board = certificate("board", "board");
  const terminal = certificate("terminal", "terminal");
  const customer = certificate("customer", "customer");
  const otherOwner = certificate("other-owner", "owner");
  const payerKey = secp256k1.utils.randomPrivateKey(), merchantKey = secp256k1.utils.randomPrivateKey();
  const payerAddress = addressOfPrivateKey(payerKey);
  const expected = Buffer.from(policy.expected);
  expected.set(Buffer.from(payerAddress.slice(2), "hex"), quoteV2Layout.payerOffset + 12);
  expected.writeBigUInt64BE(1n, quoteV2Layout.payerGenerationOffset + 24);
  const quoteNow = BigInt(Math.floor(Date.now() / 1000));
  expected.writeBigUInt64BE(quoteNow - 1n, quoteV2Layout.issuedAtOffset + 24);
  expected.writeBigUInt64BE(quoteNow + 110n, quoteV2Layout.expiresAtOffset + 24);
  const merchantAddress = Buffer.from(addressOfPrivateKey(merchantKey).slice(2), "hex");
  const freshDigest = Buffer.from(quoteDigest(expected));
  const quote = Buffer.concat([expected, Buffer.from(signDigest(freshDigest, merchantKey))]);
  const hash = (text: string) => Buffer.from(keccak_256(Buffer.from(text)));
  const domain = Buffer.from(keccak_256(Buffer.concat([
    hash(quoteV2Layout.domainType), hash(quoteV2Layout.domainName), hash(quoteV2Layout.domainVersion), expected.subarray(0, 64),
  ])));
  const approvalDigest = keccak_256(Buffer.concat([
    Buffer.of(0x19, 1), domain,
    Buffer.from(keccak_256(Buffer.concat([hash("PaymentApproval(bytes32 quoteDigest)"), freshDigest]))),
  ]));
  const quoteFile = file("quote-policy.bin");
  const quoteTime = Buffer.alloc(8);
  quoteTime.writeBigUInt64BE(quoteNow);
  writeFileSync(quoteFile, Buffer.concat([expected, merchantAddress, quoteTime, payerKey]), { mode: 0o600 });

  async function server(profile: V2Profile, own: Certificate, expected: Certificate, legacy = false, rental?: "approve"|"refuse"|"controlled",
                        controls?: {switchPath:string;displayKey:string}): Promise<number> {
    const id = { management: 1, payment: 2, nearby: 3 }[profile].toString();
    const child = spawn(peerBinary!, [ca, own.cert, own.key, expected.cert, id, quoteFile], {
      stdio: ["ignore", "pipe", "inherit"], env: { ...process.env, ...(legacy && { NU54_TEST_LEGACY_OWNER: "1" }), ...(rental && { NU54_TEST_RENTAL_REVIEW: rental }),
        ...(controls && {NU54_TEST_W10_SWITCH:controls.switchPath,NU54_TEST_W10_DISPLAY_KEY:controls.displayKey}) },
    });
    t.after(async () => {
      if (child.exitCode !== null || child.signalCode !== null) return;
      const finished = once(child, "exit");
      child.kill();
      await finished;
    });
    const [data] = await Promise.race([
      once(child.stdout, "data"),
      once(child, "exit").then(([code]) => { throw new Error(`C TLS endpoint exited before listening (${code})`); }),
    ]);
    const port = Number(String(data).trim());
    assert(port > 0);
    return port;
  }

  function exchange(port: number, profile: V2Profile, local: V2Role, own: Certificate | undefined,
                    peer: Certificate, frames: (link: V2Link) => Uint8Array[], extra: ConnectionOptions = {},
                    observe?: (request: V2Request, link: V2Link, send: (frames: Uint8Array[]) => void) => boolean): Promise<Uint8Array> {
    return new Promise((resolve, reject) => {
      const socket = connect({
        host: "127.0.0.1", port, ca: readFileSync(ca),
        ...(own && { key: readFileSync(own.key), cert: readFileSync(own.cert) }),
        minVersion: "TLSv1.3", maxVersion: "TLSv1.3", ALPNProtocols: [V2_ALPN[profile]],
        rejectUnauthorized: true, checkServerIdentity: () => undefined, secureOptions: constants.SSL_OP_NO_TICKET,
        ...extra,
      });
      const decoder = new V2StreamDecoder();
      let active: V2Link | undefined;
      let completed = false;
      function fail(error: Error) {
        socket.destroy();
        reject(error);
      }
      socket.setTimeout(3000, () => fail(new Error("TLS exchange timeout")));
      socket.on("error", fail);
      socket.once("close", () => {
        decoder.close();
        active?.close();
        if (!completed) reject(new Error("TLS exchange closed before a verified response"));
      });
      function send(fragments: Uint8Array[]) {
        for (const frame of fragments) {
          const packet = encodeV2StreamFrame(frame);
          socket.write(packet.subarray(0, 1));
          socket.write(packet.subarray(1));
        }
      }
      socket.once("secureConnect", () => {
        try {
          active = bindV2NodeTls(socket, profile, local, peer.fingerprint, new V2Journal(2));
          send(frames(active));
        } catch (error) {
          fail(error as Error);
        }
      });
      socket.on("data", (bytes: Buffer) => {
        try {
          decoder.feed(bytes, (fragment) => {
            assert(active, "expected the completed TLS handshake");
            const result = active.receive(fragment, seconds());
            if (result) {
              if (observe && !observe(result.request, active, send)) return;
              assert.equal(result.request.command, "reply");
              completed = true;
              resolve(result.request.payload);
              socket.end();
            }
          });
        } catch (error) {
          fail(error as Error);
        }
      });
    });
  }

  const managementPort = await server("management", board, owner);
  const paymentPort = await server("payment", board, terminal);
  const nearbyPort = await server("nearby", terminal, customer);
  const request: V2Request = { command: "pay.prepare", id: new Uint8Array(16).fill(7), expires: seconds() + 120n, payload: quote };
  let previous: Uint8Array[] = [];
  const relayed = await exchange(managementPort, "management", "owner", owner, board, (link) => (previous = link.send(request)));
  const direct = await exchange(paymentPort, "payment", "terminal", terminal, board, (link) => link.send(request));
  assert.deepEqual(relayed, direct);
  assert.equal(relayed[0], 1);
  assert.deepEqual(Buffer.from(relayed.subarray(1, 33)), freshDigest);
  assert.equal(recoverSigner(approvalDigest, relayed.subarray(33, 98)), payerAddress);
  assert.deepEqual(Buffer.from(relayed.subarray(98)), quote);
  const offer: V2Request = {
    command: "manage", id: new Uint8Array(16).fill(11), expires: seconds() + 10n,
    payload: encodeApprovalProgressOffer(1n),
  };
  const negotiatedPort = await server("management", board, owner);
  const observed: V2Request["command"][] = [];
  const observedResult = await exchange(negotiatedPort, "management", "owner", owner, board,
    (link) => link.send(offer), {}, (received, link, send) => {
      observed.push(received.command);
      if (observed.length === 1) {
        assert.equal(received.command, "reply");
        link.acceptApprovalProgressOffer(received.payload, 1n);
        send(link.send(request));
        return false;
      }
      if (observed.length === 2) {
        assert.equal(received.command, "approval.progress");
        assert.deepEqual(received.id, request.id);
        assert.equal(received.expires, request.expires);
        assert.deepEqual(received.payload, Uint8Array.of(1, 0, 0, 0, 0, 0, 0, 0, 1));
        assert.throws(() => link.send(offer), /outstanding/);
        return false;
      }
      return true;
    });
  assert.deepEqual(observed, ["reply", "approval.progress", "reply"]);
  assert.deepEqual(observedResult, relayed);

  const nonce = Uint8Array.from({length:32}, (_,i)=>i+1);
  const rentalRequest = {...request, id:Uint8Array.from({length:16},()=>0x29), expires:quoteNow+110n,
    payload:rentalReviewPrepare(nonce, quote)};
  const boardPoint = new X509Certificate(readFileSync(board.cert)).publicKey.export({format:"der",type:"spki"}).subarray(26);
  const payment = {bytes:quote,deadline:Number(rentalRequest.expires),payer:payerAddress,payerRole:"renter" as const,
    generation:1n,digest:bytesToHex(freshDigest,false),approvalDigest:bytesToHex(approvalDigest,false)};
  const reviewScope = {boardPin:bytesToHex(board.fingerprint,false),terminalPin:bytesToHex(terminal.fingerprint,false),
    signer:bytesToHex(boardPoint,false),request:bytesToHex(rentalRequest.id,false),challenge:bytesToHex(nonce,false)};
  const ports:number[]=[];
  for (const mode of ["refuse","approve"] as const) {
    const rentalPort = await server("payment",board,terminal,false,mode);
    ports.push(rentalPort);
    let review:Uint8Array|undefined;
    let encryptedEpoch:Uint8Array;
    const bytes = await exchange(rentalPort,"payment","terminal",terminal,board,link=>{
      const frames=link.send(rentalRequest);
      encryptedEpoch=frames[0].slice(0,16);
      return frames;
    },{},(message,link)=>{
      if(message.command!=="rental.review") return true;
      assert(!review,"duplicate review");
      review=message.payload;
      const inspected=verifyRentalPaymentReview(review,payment,{...reviewScope,now:Number(seconds())});
      assert.deepEqual(inspected.epoch,encryptedEpoch);
      assert.throws(()=>link.send(request),/outstanding/);
      return false;
    });
    assert(review);
    const expected={...reviewScope,now:Number(seconds())};
    const result=verifyRentalPaymentDecision(bytes,review,payment,expected);
    assert.equal(result.decision,mode==="approve"?"approved":"refused");
    if(mode==="approve"&&result.decision==="approved")
      assert.equal(recoverSigner(approvalDigest,Buffer.from(result.signature,"hex")),payerAddress);
    assert.throws(()=>verifyRentalPaymentDecision(Uint8Array.of(0),review!,payment,expected));
    const altered=review.slice(); altered[68]^=1;
    assert.throws(()=>verifyRentalPaymentDecision(bytes,altered,payment,expected));
    await assert.rejects(exchange(rentalPort,"payment","terminal",terminal,board,link=>link.send(rentalRequest)));
  }
  await assert.rejects(exchange(paymentPort,"payment","terminal",terminal,board,link=>link.send(rentalRequest)));
  await assert.rejects(exchange(ports[0],"payment","terminal",owner,board,link=>link.send(rentalRequest)));

  const displayKey = certificate("independent-display","owner").key;
  const switchPath = file("w10-synthetic-switch");
  for (const mode of ["refuse","approve","invalid-display"] as const) {
    const armedPort=await server("payment",board,terminal,false,"controlled",
      {switchPath,displayKey:mode==="invalid-display"?board.key:displayKey});
    let reviewed:Uint8Array|undefined;
    let switched=false;
    const decision=exchange(armedPort,"payment","terminal",terminal,board,link=>link.send(rentalRequest),{},message=>{
      if(message.command==="rental.review") {
        reviewed=message.payload;
        verifyRentalPaymentReview(reviewed,payment,{...reviewScope,now:Number(seconds())});
        setTimeout(()=>{
          writeFileSync(switchPath+".next",(mode==="invalid-display"?"approve":mode)+"\n",{mode:0o600});
          renameSync(switchPath+".next",switchPath);
          switched=true;
        },50);
        return false;
      }
      assert(switched,"the controlled host decided before the test selected a button");
      return true;
    });
    if (mode==="invalid-display") {
      await assert.rejects(decision);
      assert(switched && reviewed);
    } else {
      const wire=await decision;
      assert(reviewed);
      assert.equal(verifyRentalPaymentDecision(wire,reviewed,payment,{...reviewScope,now:Number(seconds())}).decision,
        mode==="refuse"?"refused":"approved");
    }
  }

  // Load the product implementation at runtime so the protocol's Node
  // typecheck does not import React Native ambient declarations.
  const product = new URL("../../../../products/p04-merchant-kiosk/src/kiosk/boardPayment.ts",import.meta.url);
  const {reviewRentalBoard}=await import(product.href);
  const otherBoard=certificate("other-board","board");
  const distractor=await server("payment",otherBoard,terminal,false,"approve");
  for (const handles of [["distractor","enrolled"],["enrolled","distractor"]]) {
    const correct=await server("payment",board,terminal,false,"approve");
    const streamWrites=new Map<string,number>();
    const opens:string[]=[];
    let listener:((event:string)=>void)|undefined;
    const active=new Map<string,ReturnType<typeof connect>>();
    const emit=(kind:string,link:string,extra:Record<string,unknown>={})=>
      listener?.(JSON.stringify({kind,link,...extra}));
    const branch="0fbc54ef-2827-4ab5-bfcd-3a27611c39ea";
    const adapter={
      configurePayment:async()=>{},
      scanPayment:async()=>JSON.stringify(handles.map(handle=>({handle}))),
      onPayment:(callback:(event:string)=>void)=>{
        listener=callback;
        return {remove:()=>{listener=undefined;}};
      },
      connectPayment:async(handle:string)=>{
        assert.equal(active.size,0);
        opens.push(handle);
        const expected=handle==="enrolled"?board:otherBoard;
        const socket=connect({
          host:"127.0.0.1",port:handle==="enrolled"?correct:distractor,ca:readFileSync(ca),
          key:readFileSync(terminal.key),cert:readFileSync(terminal.cert),
          minVersion:"TLSv1.3",maxVersion:"TLSv1.3",ALPNProtocols:[V2_ALPN.payment],
          rejectUnauthorized:true,checkServerIdentity:()=>undefined,secureOptions:constants.SSL_OP_NO_TICKET,
        });
        active.set(handle,socket);
        socket.once("secureConnect",()=>{
          try{
            bindV2NodeTls(socket,"payment","terminal",expected.fingerprint,new V2Journal(2));
            const epoch=socket.exportKeyingMaterial(16,V2_EXPORTER_LABEL,Buffer.from(V2_ALPN.payment));
            emit("ready",handle,{epoch:toBase64(epoch),peer:bytesToHex(expected.fingerprint,false),
              branch,principal:branch,expiresAt:Number(quoteNow+110n)});
          }catch{socket.destroy();}
        });
        socket.on("data",(bytes:Buffer)=>emit("data",handle,{clear:toBase64(bytes)}));
        socket.on("error",()=>socket.destroy());
        socket.once("close",()=>emit("closed",handle));
        return handle;
      },
      writePayment:async(handle:string,data:string)=>{
        streamWrites.set(handle,(streamWrites.get(handle)??0)+1);
        const socket=active.get(handle);
        assert(socket,"write without a native connection");
        await new Promise<void>((resolve,reject)=>socket.write(fromBase64(data),error=>error?reject(error):resolve()));
      },
      closePayment:async(handle:string)=>{
        const socket=active.get(handle); assert(socket);
        active.delete(handle);
        socket.destroy();
      },
    };
    let writes=0,acceptedReview=0;
    const signature=await reviewRentalBoard(adapter,
      "isolated enrollment",branch,payment,new AbortController().signal,{
        scope:{...reviewScope,expiresAt:Number(seconds()+12n)},begin:()=>{writes++;},reviewed:()=>{acceptedReview++;},
      });
    assert.equal(writes,1);
    assert.equal(acceptedReview,1);
    assert.equal(recoverSigner(approvalDigest,Buffer.from(signature,"hex")),payerAddress);
    assert.equal(streamWrites.get("distractor"),undefined);
    assert((streamWrites.get("enrolled")??0)>0);
    assert.deepEqual(opens,handles[0]==="enrolled"?["enrolled"]:["distractor","enrolled"]);
    assert.equal(active.size,0);
  }

  const legacyPort = await server("management", board, owner, true);
  await assert.rejects(exchange(legacyPort, "management", "owner", owner, board, (link) => link.send(offer)));
  // A fresh authenticated legacy link sends the original only after the idle probe closed.
  assert.deepEqual(await exchange(legacyPort, "management", "owner", owner, board, (link) => link.send(request)), relayed);

  const intent = Buffer.from("synthetic nearby intent");
  assert.equal((await exchange(nearbyPort, "nearby", "customer", customer, terminal,
    (link) => link.send({ ...request, command: "nearby.submit", payload: intent })))[0], 1);

  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, () => previous));
  const resumed = await exchange(managementPort, "management", "owner", owner, board, (link) => {
    const frames = link.send(request);
    assert.notDeepEqual(frames[0].subarray(0, 16), previous[0].subarray(0, 16));
    return frames;
  });
  assert.equal(resumed[0], 2);
  assert.deepEqual(resumed.subarray(1), relayed.subarray(1));

  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) =>
    link.send({ ...request, payload: new Uint8Array([1]) })));
  const wrongQuote = Buffer.from(quote);
  wrongQuote[wrongQuote.length - 2] ^= 1;
  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) =>
    link.send({ ...request, id: Buffer.alloc(16, 8), payload: wrongQuote })));
  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) =>
    link.send({ ...request, expires: seconds() })));
  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) =>
    link.send(request).reverse()));
  await assert.rejects(exchange(managementPort, "management", "owner", otherOwner, board, (link) => link.send(request)));
  await assert.rejects(exchange(managementPort, "management", "owner", undefined, board, (link) => link.send(request)));
  await assert.rejects(exchange(managementPort, "management", "owner", owner, terminal, (link) => link.send(request)), /pin/);
  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) => link.send(request),
    { ALPNProtocols: ["nu54-v1"] }));
  await assert.rejects(exchange(managementPort, "management", "owner", owner, board, (link) => link.send(request),
    { minVersion: "TLSv1.2", maxVersion: "TLSv1.2" }));
  const wrongRolePort = await server("management", board, terminal);
  await assert.rejects(exchange(wrongRolePort, "management", "owner", terminal, board, (link) => link.send(request)));
});
