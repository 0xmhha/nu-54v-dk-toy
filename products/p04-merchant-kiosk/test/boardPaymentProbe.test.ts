import {fromBase64,hexToBytes,toBase64,V2Journal,V2Link,V2StreamDecoder,
  type VerifiedCommercePayment} from '@nu54/protocol';
import {reviewRentalBoard} from '../src/kiosk/boardPayment';
import type {Spec} from '../src/specs/NativeNearby';

const branch='0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';
const signer='8c'+'24'.repeat(64);
const boardPin='23'.repeat(32);
const terminalPin='43'.repeat(32);
const otherPin='17'.repeat(32);
const epoch=new Uint8Array(16).fill(5);
const expiration=1800000000;
const scope={boardPin,terminalPin,signer,request:'a9'.repeat(16),challenge:'47'.repeat(32),expiresAt:expiration+13};
const payment:VerifiedCommercePayment={bytes:new Uint8Array(737).fill(0x4f),payer:'0x'+'50'.repeat(20),
  payerRole:'renter',generation:1n,deadline:expiration+60,digest:'',approvalDigest:''};
type Candidate={handle:string;pin?:string;closedBeforeReturn?:boolean};

function nativeFixture(candidates:Candidate[]) {
  let listen:((raw:string)=>void)|undefined;
  let active:string|undefined;
  const ready=(link:string,pin:string)=>JSON.stringify({kind:'ready',link,peer:pin,principal:branch,branch,
    epoch:toBase64(epoch),expiresAt:expiration+60});
  const board=new V2Link('payment','board',epoch.slice(),hexToBytes(terminalPin),new V2Journal(3));
  const stream=new V2StreamDecoder();
  const attempts:string[]=[];
  const writes=jest.fn(async (link:string,clear:string)=>{
    expect(active).toBe(link);
    stream.feed(fromBase64(clear),frame=>{
      const request=board.receive(frame,BigInt(expiration))?.request;
      if(!request)return;
      expect(request.command).toBe('pay.prepare');
      expect(request.id).toEqual(hexToBytes(scope.request));
      expect(request.payload.subarray(36)).toEqual(payment.bytes);
      attempts.push(link);
      listen?.(JSON.stringify({kind:'closed',link}));
    });
  });
  const module={
    configurePayment:jest.fn(async()=>{}),
    scanPayment:jest.fn(async()=>JSON.stringify(candidates.map(({handle})=>({handle})))),
    connectPayment:jest.fn(async(handle:string)=>{
      expect(active).toBeUndefined();
      const candidate=candidates.find(item=>item.handle===handle)!;
      if(!candidate.pin)throw new Error('NATIVE_TLS_UNAVAILABLE');
      active=handle;
      listen?.(ready(handle,candidate.pin));
      if(candidate.closedBeforeReturn)listen?.(JSON.stringify({kind:'closed',link:handle}));
      return handle;
    }),
    closePayment:jest.fn(async(link:string)=>{expect(active).toBe(link);active=undefined;}),
    writePayment:writes,
    onPayment:jest.fn((callback:(raw:string)=>void)=>{
      expect(listen).toBeUndefined();
      listen=callback;
      return {remove:()=>{listen=undefined;}};
    }),
  };
  return {module,attempts,writes};
}

test.each([
  ['wrong first',[{handle:'same-name-wrong',pin:otherPin},{handle:'correct',pin:boardPin}]],
  ['correct first',[{handle:'correct',pin:boardPin},{handle:'same-name-wrong',pin:otherPin}]],
])('%s: only the bonded pin receives one logical original request',async(_title,candidates)=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(expiration*1000);
  const {module,attempts,writes}=nativeFixture(candidates as Candidate[]);
  const start=jest.fn();
  try {
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin:start,reviewed:()=>{}})).rejects.toThrow('PAYMENT_RESULT_UNKNOWN');
    expect(attempts).toEqual(['correct']);
    expect(writes.mock.calls.every(([handle])=>handle==='correct')).toBe(true);
    expect(start).toHaveBeenCalledTimes(1);
    expect(module.connectPayment.mock.calls.map(([handle])=>handle)).toEqual(
      candidates[0].handle==='correct'?['correct']:['same-name-wrong','correct']);
  }finally{time.mockRestore();}
});

test('TLS errors or already closed ready events remain pre-write, and expiry prevents further probes',async()=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(expiration*1000);
  const {module,attempts,writes}=nativeFixture([
    {handle:'tls-rejected'},{handle:'closed',pin:boardPin,closedBeforeReturn:true},{handle:'correct',pin:boardPin},
  ]);
  const begin=jest.fn();
  try {
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin,reviewed:()=>{}})).rejects.toThrow('PAYMENT_RESULT_UNKNOWN');
    expect(attempts).toEqual(['correct']);
    expect(begin).toHaveBeenCalledTimes(1);
    writes.mockClear();
    module.closePayment.mockImplementationOnce(async()=>{time.mockReturnValue(scope.expiresAt*1000);});
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin,reviewed:()=>{}})).rejects.toThrow('PAYMENT_EXPIRY');
    expect(writes).not.toHaveBeenCalled();
  }finally{time.mockRestore();}
});

test('a rejected first native write is uncertain and never probes or sends to any other candidate',async()=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(expiration*1000);
  const {module,writes}=nativeFixture([{handle:'correct',pin:boardPin},{handle:'other',pin:otherPin}]);
  writes.mockRejectedValueOnce(new Error('NATIVE_WRITE_LOST'));
  try {
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin:()=>{},reviewed:()=>{}})).rejects.toThrow('PAYMENT_RESULT_UNKNOWN');
    expect(module.connectPayment.mock.calls).toEqual([['correct']]);
    expect(writes).toHaveBeenCalledTimes(1);
  }finally{time.mockRestore();}
});

const sameNameBoards = Array.from({length:15},(_,index)=>({
  handle:'independent-'+index,pin:otherPin,name:'Shared board name',
}));
const originalBoard={handle:'correct',pin:boardPin,name:'Shared board name'};

test.each([
  ['original last',[...sameNameBoards,originalBoard]],
  ['original first',[originalBoard,...sameNameBoards]],
])('16 native candidates, %s: only the original receives the request',async(_title,candidates)=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(expiration*1000);
  const {module,attempts,writes}=nativeFixture(candidates);
  module.scanPayment.mockResolvedValueOnce(JSON.stringify(candidates));
  const begin=jest.fn();
  try {
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin,reviewed:()=>{}})).rejects.toThrow('PAYMENT_RESULT_UNKNOWN');
    expect(attempts).toEqual(['correct']);
    expect(begin).toHaveBeenCalledTimes(1);
    expect(writes.mock.calls.map(([handle])=>handle).every(handle=>handle==='correct')).toBe(true);
    expect(module.connectPayment).toHaveBeenCalledTimes(candidates[0].handle==='correct'?1:16);
  }finally{time.mockRestore();}
});

test('17 native candidates are rejected before connecting or starting any payment',async()=>{
  const time=jest.spyOn(Date,'now').mockReturnValue(expiration*1000);
  const {module,writes}=nativeFixture([...sameNameBoards,originalBoard,{handle:'overflow',pin:otherPin}]);
  const begin=jest.fn();
  try {
    await expect(reviewRentalBoard(module as unknown as Spec,'enrollment',branch,payment,new AbortController().signal,
      {scope,begin,reviewed:()=>{}})).rejects.toThrow('PAYMENT_BOARD_SCAN');
    expect(module.connectPayment).not.toHaveBeenCalled();
    expect(begin).not.toHaveBeenCalled();
    expect(writes).not.toHaveBeenCalled();
  }finally{time.mockRestore();}
});
