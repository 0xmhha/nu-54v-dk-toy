/// <reference types="node" />
import {readFileSync} from 'node:fs';
import {resolve as resolvePath} from 'node:path';
import {ed25519} from '@noble/curves/ed25519';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {
  addressOfPrivateKey, bigToBytes, bytesToBig, bytesToHex, concat, decodeNearbyRequest, hexToBytes, packNearbyPresentation,
  presentationMessage, quoteDigest, quoteV2Layout as q, rentalReviewAuthorityMessage, signDigest, toBase64, utf8Encode,
  verifyRentalPaymentReview,
  type CommerceClient, type CommerceOrder, type CommerceQuote, type NearbyReady, type RentalReviewAuthority,
  type VerifiedCommercePayment,
} from '@nu54/protocol';
import {NearbyRelay} from '../src/kiosk/nearbyRelay';
import type {MerchantInbox} from '../src/kiosk/merchantInbox';

const path = resolvePath(__dirname,'../../../packages/protocol/v2/testdata/quote.vectors');
const [,,, unsigned,, clockText,, digest] = readFileSync(path,'utf8').split('\n')[1].split('|');
const clock = Number(clockText);
const raw = hexToBytes(unsigned);
const word = (offset:number) => raw.subarray(offset,offset+32);
const addr = (offset:number) => bytesToHex(word(offset).subarray(12));
const value = (offset:number) => bytesToBig(word(offset));
const merchantKey = new Uint8Array(32).fill(9);
const signed = bytesToHex(concat(raw,signDigest(quoteDigest(raw),merchantKey)),false);
const authorityKey = new Uint8Array(32).fill(4);
const boardKey = new Uint8Array(32).fill(7);
const boardPublic = p256.getPublicKey(boardKey,false);
const boardPin = sha256(concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'),boardPublic));
const terminalPin = new Uint8Array(32).fill(3);
const epoch = new Uint8Array(16).fill(5);
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';
const customer = 'b3c6efeb-b8f7-48ae-9238-a2fe9cf8632b';
const orderID = 'f0551c76-7592-4850-a414-54464d24dafe';
const key = '91554c08-6550-4f08-a47e-8d85503599c1';
const proof:RentalReviewAuthority = {
  request:'27'.repeat(16),challenge:'37'.repeat(32),expiresAt:clock+15,
  boardPin:bytesToHex(boardPin,false),terminalPin:bytesToHex(terminalPin,false),signer:bytesToHex(boardPublic,false),signature:'',
};
const quote:CommerceQuote = {
  saleRef:orderID,branchRef:branch,orderKey:bytesToHex(word(q.orderIdOffset),false),
  expiresAt:Number(value(q.expiresAtOffset)), quoteDigest:digest,signedQuote:signed,
  presentation:{customer,origin:'488009c9-c4ab-41e6-a2ad-9124692e25bb',claim:'43'.repeat(32),signature:''},
};
quote.presentation!.signature = bytesToHex(ed25519.sign(presentationMessage(quote,value(q.payerGenerationOffset)),authorityKey),false);
proof.signature = bytesToHex(ed25519.sign(rentalReviewAuthorityMessage({...quote,presentation:{...quote.presentation!,rentalReview:proof}},
  value(q.payerGenerationOffset)),authorityKey),false);
const profile = {
  chain:8283 as const, contract:addr(q.verifyingContractOffset),asset:addr(q.assetOffset),beneficiary:addr(q.beneficiaryOffset),
  merchantId:bytesToHex(word(q.merchantIdOffset),false),signer:addressOfPrivateKey(merchantKey),
  keyVersion:Number(value(q.merchantKeyVersionOffset)),beneficiaryVersion:Number(value(q.beneficiaryVersionOffset)),
  maxBaseUnits:'1000000', presentationKey:bytesToHex(ed25519.getPublicKey(authorityKey),false),
};
const order:CommerceOrder = {
  id:orderID,revision:3,state:'issued',asset:profile.asset,amount:value(q.amountOffset).toString(),orderKey:quote.orderKey,
  expiresAt:new Date(quote.expiresAt*1000).toISOString(),quoteDigest:digest,
};
const peer:NearbyReady = {kind:'ready',principal:customer,branch,link:'isolated-customer-C3',epoch:toBase64(epoch),
  peer:'53'.repeat(32),expiresAt:clock+30};
const source = {op:'rental-review-source',branch,key,order:orderID,value:packNearbyPresentation(order,quote,toBase64)} as const;
const inspection = {op:'rental-review',branch,key,order:orderID,request:proof.request,challenge:proof.challenge,
  quoteDigest:digest,authority:proof} as const;

function boardReceipt(payment:VerifiedCommercePayment,scope=proof) {
  const statement = concat(utf8Encode('W10R'),boardPin,terminalPin,epoch,hexToBytes(scope.request),hexToBytes(scope.challenge),
    payment.bytes.subarray(q.orderIdOffset,q.orderIdOffset+32),sha256(payment.bytes),hexToBytes(payment.payer),bigToBytes(payment.generation,8),bigToBytes(BigInt(payment.deadline),8));
  return bytesToHex(concat(statement,p256.sign(sha256(concat(utf8Encode('NU54 W10 rental payment review v1\0'),statement)),
    boardKey,{lowS:true,prehash:false}).toCompactRawBytes()),false);
}

function createRelay() {
  const status = jest.fn(async (_id?:string):Promise<CommerceOrder> => order);
  const write = jest.fn(() => {throw Error('unexpected order or payment write');});
  const api = {profile:{branchRef:branch},status,reserve:write,authorize:write,quote:write,forward:write,nearbyReservation:write};
  const read = jest.fn(async (payment:VerifiedCommercePayment,_scope:RentalReviewAuthority,_signal:AbortSignal,_decided:(wire:string)=>void,begin:()=>void) => {
    begin();
    return boardReceipt(payment,_scope);
  });
  const relay = new NearbyRelay(api as unknown as CommerceClient,{} as MerchantInbox,async()=>new Uint8Array(16),
    ()=>undefined,()=>{},undefined,{policy:profile,read});
  relay.activate(peer.link);
  return {relay,status,write,read};
}

test('direct QR or W09 original reaches a fresh terminal without checkout, then exports exactly one review',async()=>{
  const time = jest.spyOn(Date,'now').mockReturnValue(clock*1000);
  const {relay,write,read,status} = createRelay();
  try{
    expect(await relay.handle(inspection,peer)).toEqual({status:'denied'});
    const body = utf8Encode(JSON.stringify(source));
    expect(body.length).toBeLessThan(1900);
    expect(await relay.handle(decodeNearbyRequest(body,branch),peer)).toEqual({status:'ok'});
    const reply = await relay.handle(inspection,peer);
    expect(reply.status).toBe('ok');
    const receipt = (reply.value as {receipt:string}).receipt;
    const payment = read.mock.calls[0][0];
    expect(verifyRentalPaymentReview(hexToBytes(receipt),payment,{...proof,now:clock})).toEqual({epoch,expires:payment.deadline});
    expect(await relay.handle(inspection,peer)).toEqual(reply);
    expect(read).toHaveBeenCalledTimes(1);
    expect(status).toHaveBeenCalledWith(orderID);
    expect(write).not.toHaveBeenCalled();
  }finally{relay.close();time.mockRestore();}
});

test('wrong buyer, stale authority, closed order, and retired read produce no fresh board operation',async()=>{
  const time = jest.spyOn(Date,'now').mockReturnValue(clock*1000);
  const {relay,read,status,write} = createRelay();
  try{
    const other = {...peer,principal:branch};
    expect((await relay.handle(source,other)).status).not.toBe('ok');
    expect(await relay.handle(source,peer)).toEqual({status:'ok'});
    expect((await relay.handle({...inspection,challenge:'a9'.repeat(32)},peer)).status).not.toBe('ok');
    status.mockResolvedValueOnce({...order,state:'closed'});
    expect((await relay.handle(inspection,peer)).status).not.toBe('ok');
    let finish:(value:CommerceOrder)=>void=()=>{};
    status.mockImplementationOnce(()=>new Promise(resolveStatus=>{finish=resolveStatus;}));
    const late = relay.handle(inspection,peer);
    relay.retire(peer.link);
    finish(order);
    expect((await late).status).not.toBe('ok');
    expect(read).not.toHaveBeenCalled();
    expect(write).not.toHaveBeenCalled();
  }finally{relay.close();time.mockRestore();}
});


function anotherOriginal() {
  const otherOrder = '56ab2ae2-b515-4e78-a187-8ecf14dd39f3';
  const other = {...peer,link:'independent-C3',principal:'4e3c9b45-2ba5-4c13-90bf-8f48a4a7cfce'};
  const otherRaw = raw.slice();
  otherRaw[q.orderIdOffset+31] = (otherRaw[q.orderIdOffset+31]+1)%256;
  const otherDigest = quoteDigest(otherRaw);
  const otherQuote:CommerceQuote = {...quote,saleRef:otherOrder,quoteDigest:bytesToHex(otherDigest,false),
    orderKey:bytesToHex(otherRaw.subarray(q.orderIdOffset,q.orderIdOffset+32),false),
    signedQuote:bytesToHex(concat(otherRaw,signDigest(otherDigest,merchantKey)),false),
    presentation:{...quote.presentation!,customer:other.principal}};
  otherQuote.presentation!.signature = bytesToHex(ed25519.sign(
    presentationMessage(otherQuote,value(q.payerGenerationOffset)),authorityKey),false);
  const otherStatus:CommerceOrder = {...order,id:otherOrder,orderKey:otherQuote.orderKey,quoteDigest:otherQuote.quoteDigest};
  const secondProof:RentalReviewAuthority={...proof,request:'59'.repeat(16),challenge:'5a'.repeat(32),signature:''};
  secondProof.signature=bytesToHex(ed25519.sign(rentalReviewAuthorityMessage({
    ...otherQuote,presentation:{...otherQuote.presentation!,rentalReview:secondProof},
  },value(q.payerGenerationOffset)),authorityKey),false);
  return {otherOrder,other,otherStatus,
    source:{...source,order:otherOrder,value:packNearbyPresentation(otherStatus,otherQuote,toBase64)},
    inspection:{...inspection,order:otherOrder,request:secondProof.request,challenge:secondProof.challenge,
      quoteDigest:otherQuote.quoteDigest,authority:secondProof}};
}

test('idle expiry releases the receipt and replay fence, with immediate cleanup on link retirement',async()=>{
  jest.useFakeTimers();
  jest.setSystemTime(clock*1000);
  const {relay,read,status} = createRelay();
  const next=anotherOriginal();
  const {other,otherOrder,otherStatus}=next;
  status.mockImplementation(async (id?:string)=>id===otherOrder?otherStatus:order);
  const counts=relay as unknown as {quotes:Map<string,unknown>;reviews:Map<string,unknown>;reviewFences:Map<string,unknown>};
  try{
    relay.activate(other.link);
    expect(await relay.handle(next.source,other))
      .toEqual({status:'ok'});
    expect(await relay.handle(source,peer)).toEqual({status:'ok'});
    expect((await relay.handle(inspection,peer)).status).toBe('ok');
    expect(counts.reviews.size).toBe(1);
    expect(counts.quotes.size).toBe(2);
    relay.retire(other.link);
    expect(counts.quotes.has(otherOrder)).toBe(false);
    expect(counts.reviews.has(orderID)).toBe(true);
    await jest.advanceTimersByTimeAsync(15000);
    expect(counts.reviews.size).toBe(0);
    expect(counts.quotes.size).toBe(0);
    expect(counts.reviewFences.has(orderID)).toBe(true);
    expect(read.mock.calls[0][2].aborted).toBe(true);
    await jest.advanceTimersByTimeAsync((quote.expiresAt-clock-15)*1000);
    expect(counts.reviewFences.size).toBe(0);
  }finally{relay.close();jest.useRealTimers();}
});


test('the original customer reads verified SW2 only, with no writes or new board action after retirement',async()=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(clock*1000);
  const {relay,read,write,status}=createRelay();
  try {
    expect(await relay.handle(source,peer)).toEqual({status:'ok'});
    const review=(await relay.handle(inspection,peer)).value as {receipt:string};
    const bytes=hexToBytes(review.receipt);
    const reviewDigest=bytesToHex(sha256(bytes),false);
    const poll={...inspection,op:'rental-decision' as const,reviewDigest};
    expect(await relay.handle(poll,peer)).toEqual({status:'pending'});
    const deliver=read.mock.calls[0][3];
    deliver('00');
    expect(await relay.handle(poll,peer)).toEqual({status:'pending'});
    const body=concat(utf8Encode('W10D'),sha256(bytes),Uint8Array.of(2),new Uint8Array(65));
    const final=bytesToHex(concat(body,p256.sign(sha256(concat(utf8Encode('NU54 W10 rental payment decision v1\0'),
      body)),boardKey,{lowS:true,prehash:false}).toCompactRawBytes()),false);
    deliver(final);
    expect(await relay.handle({...poll,reviewDigest:'a1'.repeat(32)},peer)).toEqual({status:'denied'});
    expect(await relay.handle(poll,{...peer,principal:branch})).toEqual({status:'denied'});
    expect(await relay.handle(poll,peer)).toEqual({status:'ok',value:{decision:final}});
    expect(await relay.handle(poll,peer)).toEqual({status:'ok',value:{decision:final}});
    status.mockResolvedValueOnce({...order,state:'closed'});
    expect(await relay.handle(poll,peer)).toEqual({status:'denied'});
    relay.retire(peer.link);
    expect(await relay.handle(poll,peer)).toEqual({status:'denied'});
    const newLink={...peer,link:'replacement-link'};
    relay.activate(newLink.link);
    expect(await relay.handle(source,newLink)).toEqual({status:'ok'});
    expect(await relay.handle(inspection,newLink)).toEqual({status:'denied'});
    expect(read).toHaveBeenCalledTimes(1);
    expect(write).not.toHaveBeenCalled();
  }finally{relay.close();time.mockRestore();}
});


test('a busy terminal does not burn the other customer’s original before the first write',async()=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(clock*1000);
  const {relay,read,status,write}=createRelay();
  const next=anotherOriginal();
  const {other,otherOrder,otherStatus}=next;
  status.mockImplementation(async id=>id===otherOrder?otherStatus:order);
  let releaseFirst:()=>void=()=>{};
  let signalFirst:()=>void=()=>{};
  const firstStarted=new Promise<void>(resolve=>{signalFirst=resolve;});
  read.mockImplementationOnce(async (payment,_scope,_signal,_decided,begin)=>{
    begin();
    signalFirst();
    await new Promise<void>(resolve=>{releaseFirst=resolve;});
    return boardReceipt(payment,_scope);
  }).mockRejectedValueOnce(new Error('PAYMENT_SCOPE'));
  try {
    relay.activate(other.link);
    expect(await relay.handle(source,peer)).toEqual({status:'ok'});
    expect(await relay.handle(next.source,other)).toEqual({status:'ok'});
    const first=relay.handle(inspection,peer);
    await firstStarted;
    expect(await relay.handle(next.inspection,other)).toEqual({status:'pending'});
    releaseFirst();
    expect((await first).status).toBe('ok');
    expect((await relay.handle(next.inspection,other)).status).toBe('ok');
    expect(read.mock.calls.map(call=>call[1].challenge)).toEqual([
      inspection.challenge,next.inspection.challenge,next.inspection.challenge,
    ]);
    expect(write).not.toHaveBeenCalled();
  }finally{releaseFirst();relay.close();time.mockRestore();}
});
