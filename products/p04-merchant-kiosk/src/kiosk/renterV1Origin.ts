import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,utf8Encode} from '@nu54/protocol';
import Native from '../specs/NativeCommerceHttp.ts';
import type {Authorization} from '../chain/settlement.ts';
import type {Signed} from '../payment/submit.ts';

export interface RenterV1Origin {
  origin:string;
  profileDigest:string;
  merchantSignature:string;
}
export interface RenterV1Observation {
  reserve: (account:string, order:Omit<Authorization,'nonce'>, merchantSignature:string)=>Promise<RenterV1Origin|undefined>;
  report: (origin:RenterV1Origin, signed:Signed)=>Promise<boolean>;
}

const ref=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const payload=(account:string,order:Omit<Authorization,'nonce'>,merchantSignature:string)=>({
  account:account.toLowerCase(),orderKey:order.orderId.toLowerCase(),token:order.token.toLowerCase(),
  amount:order.amount,expiry:order.expiry,merchantSignature:merchantSignature.toLowerCase(),
});

export const renterV1Observation:RenterV1Observation={
  async reserve(account,order,merchantSignature){
    if(!Native)return;
    const profile=await Native.loadProfile('terminal');
    if(!profile)return;
    const profileDigest=bytesToHex(sha256(utf8Encode(profile)),false);
    const result=JSON.parse(await Native.exchangeRenterV1Origin('reserve',profileDigest,JSON.stringify(payload(account,order,merchantSignature))));
    if(typeof result?.origin==='string' && ref.test(result.origin))return {origin:result.origin,profileDigest,merchantSignature};
  },
  async report(known,signed){
    if(!Native)return false;
    const fields={...payload(signed.device,signed.auth,known.merchantSignature),origin:known.origin,
      nonce:signed.auth.nonce,approvalSignature:signed.signature.toLowerCase()};
    const reply=JSON.parse(await Native.exchangeRenterV1Origin('confirm',known.profileDigest,JSON.stringify(fields)));
    return reply?.recorded===true;
  },
};

// Observation never owns the BLE signing or settlement path. A late reserve
// response is deliberately discarded so it cannot bind an already sent order.
export async function boundedObservation<T>(run:()=>Promise<T>):Promise<T|undefined>{
  let timer:ReturnType<typeof setTimeout>|undefined;
  try{
    return await Promise.race([
      run().catch(()=>undefined),
      new Promise<undefined>(resolve=>{timer=setTimeout(()=>resolve(undefined),350);}),
    ]);
  }finally{clearTimeout(timer);}
}
