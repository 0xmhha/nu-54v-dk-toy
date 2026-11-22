// The installed fixture's test-only P04 terminal transport to the C2 host board.
import { spawn } from "node:child_process";
import { X509Certificate } from "node:crypto";
import { once } from "node:events";
import { readFileSync, writeFileSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { createInterface } from "node:readline";
import { connect } from "node:tls";
import { bytesToHex, hexToBytes } from "../../../packages/protocol/ts/src/bytes.ts";
import { rentalReviewPrepare, verifyRentalPaymentDecision, verifyRentalPaymentReview } from "../../../packages/protocol/ts/src/rental-payment-review.ts";
import { addressOfPrivateKey } from "../../../packages/protocol/ts/src/signature.ts";
import { bindV2NodeTls, v2SpkiSha256 } from "../../../packages/protocol/ts/src/transport-v2-node.ts";
import { encodeV2StreamFrame, V2_ALPN, V2Journal, V2StreamDecoder } from "../../../packages/protocol/ts/src/transport-v2.ts";
import { quoteV2Layout } from "../../../packages/protocol/ts/src/quote-v2.ts";
import type { RentalReviewAuthority } from "../../../packages/protocol/ts/src/private-presentation.ts";
import type { VerifiedCommercePayment } from "../../../packages/protocol/ts/src/commerce-payment.ts";

const [binary, fixture] = process.argv.slice(2);
if (!isAbsolute(binary ?? "") || !isAbsolute(fixture ?? "")) throw new Error("PRIVATE_W10_FIXTURE_REQUIRED");
const file = (name: string) => join(fixture, name);
const now = () => Math.floor(Date.now() / 1000);
const input = createInterface({input: process.stdin});
const [line] = await once(input, "line");
input.close();
process.stdin.pause();
const {authority, quote, signer, payment} = JSON.parse(line) as {
  authority: RentalReviewAuthority; quote: string; signer: string;
  payment: Pick<VerifiedCommercePayment, "deadline" | "payer" | "digest" | "approvalDigest"> & {generation: string};
};
const bytes = hexToBytes(quote);
const payer = readFileSync(file("payer.key"));
if (bytes.length !== quoteV2Layout.unsignedSize + quoteV2Layout.signatureSize || !/^0x[0-9a-f]{40}$/.test(signer) ||
    payment.deadline <= now() || authority.expiresAt <= now() || payment.payer !== addressOfPrivateKey(payer))
  throw new Error("W10_FIXTURE_QUOTE");
const verified: VerifiedCommercePayment = {...payment, bytes, payerRole: "renter", generation: BigInt(payment.generation)};
const ca = readFileSync(file("ca.pem"));
const terminalCert = readFileSync(file("terminal.pem"));
const boardPin = v2SpkiSha256(new X509Certificate(readFileSync(file("board.pem"))));
const terminalPin = v2SpkiSha256(new X509Certificate(terminalCert));
if (authority.boardPin !== bytesToHex(boardPin, false) || authority.terminalPin !== bytesToHex(terminalPin, false))
  throw new Error("W10_FIXTURE_COMMISSIONING");
const clock = Buffer.alloc(8);
clock.writeBigUInt64BE(BigInt(now()));
const policy = file("policy.bin");
writeFileSync(policy, Buffer.concat([bytes.subarray(0,quoteV2Layout.unsignedSize), hexToBytes(signer), clock, payer]),
  {mode:0o600, flag:"wx"});
const board = spawn(binary, [file("ca.pem"), file("board.pem"), file("board.key"), file("terminal.pem"), "2", policy], {
  stdio:["ignore","pipe","inherit"],
  env:{PATH:process.env.PATH ?? "/usr/bin:/bin", NU54_TEST_RENTAL_REVIEW:"controlled",
    NU54_TEST_W10_SWITCH:file("switch"), NU54_TEST_W10_DISPLAY_KEY:file("display.key")},
});
try {
  const output = createInterface({input:board.stdout});
  const [portLine] = await Promise.race([once(output,"line"),once(board,"exit").then(()=>{throw new Error("W10_BOARD_START");})]);
  output.close();
  const port = Number(portLine);
  if (!Number.isInteger(port) || port<=0 || port>65535) throw new Error("W10_BOARD_PORT");
  await new Promise<void>((resolve,reject)=>{
    const socket=connect({host:"127.0.0.1",port,ca,cert:terminalCert,key:readFileSync(file("terminal.key")),
      minVersion:"TLSv1.3",maxVersion:"TLSv1.3",ALPNProtocols:[V2_ALPN.payment],checkServerIdentity:()=>undefined});
    const decoder=new V2StreamDecoder();
    let link: ReturnType<typeof bindV2NodeTls> | undefined;
    let review: Uint8Array | undefined;
    let epoch: string | undefined;
    let finished=false;
    const fail=(error:Error)=>{socket.destroy();reject(error);};
    socket.setTimeout((authority.expiresAt-now())*1000,()=>fail(new Error("W10_BOARD_TIMEOUT")));
    socket.on("error",fail);
    socket.on("close",()=>{
      decoder.close();
      link?.close();
      if(!finished) reject(new Error("W10_BOARD_CLOSED"));
    });
    socket.once("secureConnect",()=>{
      try {
        link=bindV2NodeTls(socket,"payment","terminal",boardPin,new V2Journal(2));
        const frames=link.send({command:"pay.prepare",id:hexToBytes(authority.request),expires:BigInt(payment.deadline),
          payload:rentalReviewPrepare(hexToBytes(authority.challenge),bytes)});
        epoch=bytesToHex(frames[0].subarray(0,16),false);
        for(const frame of frames) socket.write(encodeV2StreamFrame(frame));
      } catch(error) {fail(error as Error);}
    });
    socket.on("data",(chunk:Buffer)=>{
      try {
        decoder.feed(chunk,fragment=>{
          const message=link?.receive(fragment,BigInt(now()))?.request;
          if(!message) return;
          if(message.command==="rental.review" && !review) {
            const inspected=verifyRentalPaymentReview(message.payload,verified,{...authority,now:now()});
            if(bytesToHex(inspected.epoch,false)!==epoch) throw new Error("W10_BOARD_EPOCH");
            review=message.payload;
            console.log(JSON.stringify({receipt:bytesToHex(review,false)}));
          } else if(message.command==="reply" && review) {
            verifyRentalPaymentDecision(message.payload,review,verified,{...authority,now:now()});
            console.log(JSON.stringify({decision:bytesToHex(message.payload,false)}));
            finished=true;
            socket.end();
            resolve();
          } else throw new Error("W10_BOARD_RESPONSE");
        });
      } catch(error) {fail(error as Error);}
    });
  });
} finally {
  if(board.exitCode===null && board.signalCode===null) {
    board.kill();
    await once(board,"exit");
  }
}
