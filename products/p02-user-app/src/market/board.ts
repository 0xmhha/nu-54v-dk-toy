import {bytesToBig, bytesToHex, hexToBytes} from '@nu54/protocol';
import NativeOwner from '../specs/NativeOwnerWallet.ts';
import Ble from '../specs/NativeRenterBle.ts';
import {httpsEvmRpc} from '../wallet/evmTransfer.ts';
import {OwnerSession} from '../wallet/ownerSession.ts';
import {OWNER_GATT} from '../wallet/ownerWire.ts';
import {decodeWalletStorage} from '../wallet/profile.ts';
import {verifyMarketSigned, type CloudTransfer} from '../wallet/cloud/client.ts';
import type {Approval,Profile} from './lab.ts';

async function enrollment() {
  if (!NativeOwner) throw new Error('LAB_BOARD_UNAVAILABLE');
  const stored=decodeWalletStorage(await NativeOwner.loadProfile());
  if (!stored.verified || !stored.enrollment || !stored.evmRegistry ||
      stored.verified.boardFingerprint!==stored.enrollment.device) throw new Error('LAB_BOARD_ENROLLMENT');
  return {native:NativeOwner,owner:stored.verified,certificate:stored.enrollment,registry:stored.evmRegistry};
}

export type LabBoard={fingerprint:string;generation:number;user:string};

async function connect<T>(handle:string, run:(session:OwnerSession, installed:Awaited<ReturnType<typeof enrollment>>)=>Promise<T>) {
  const installed=await enrollment();
  const active=new OwnerSession(Ble,installed.native,installed.certificate);
  try {
    await active.connect(handle);
    return await run(active,installed);
  } finally {
    active.close();
  }
}

export async function scanLabBoards() {
  await enrollment();
  const devices=new Set<string>();
  const scan=Ble.onScan(packet=>{
    try {
      const found=JSON.parse(packet) as {address?:string};
      if (typeof found.address==='string' && found.address.length<=64) devices.add(found.address);
    } catch {}
  });
  try { await Ble.scanOwner(OWNER_GATT.service,6); }
  finally { scan.remove(); }
  return [...devices];
}

export async function readLabBoard(handle:string):Promise<LabBoard> {
  return connect(handle,async (active,{owner})=>({
    fingerprint:owner.boardFingerprint,generation:owner.generation,
    user:await active.marketAddress(owner.address,BigInt(owner.generation)),
  }));
}

function readGrant(value:unknown, transfer:CloudTransfer, p:Profile, deadline:number) {
  const m=transfer.market;
  if (!m || typeof value!=='string' || !/^(?:[a-f0-9]{1492}|[a-f0-9]{1628}|[a-f0-9]{1692})$/.test(value)) throw new Error('LAB_BOARD_GRANT');
  const wire=hexToBytes(value);
  const word=(start:number,end:number)=>bytesToHex(wire.subarray(start,end),true).toLowerCase();
  const mode = wire.length===846 ? 2 : wire.length===814 ? 1 : 0;
  const number=(start:number,end:number)=>bytesToBig(wire.subarray(start,end));
  if (wire[0]!==21 || number(1,9)>BigInt(deadline) || word(17,49)!==p.genesis.toLowerCase() ||
      word(49,81)!==p.deploymentHash.toLowerCase() || word(81,101)!==p.venue.address.toLowerCase() ||
      word(101,121)!==p.base.address.toLowerCase() || word(121,141)!==p.quote.address.toLowerCase() ||
      word(141,161)!==p.quoteSigner.toLowerCase() || word(161,193)!==p.venue.codeHash.toLowerCase() ||
      word(193,213)!==transfer.to || number(213,221)!==BigInt(transfer.nonce) || number(221,229)!==BigInt(transfer.maxFeeWei) ||
      wire[229]!==mode || word(230,682)!==m.offer ||
      (mode ? word(682,wire.length-64) : m.offer)!==m.calldata) throw new Error('LAB_BOARD_TERMS');
  return wire;
}

export async function signLabBoard(handle:string,bound:LabBoard, profile:Profile, transfer:CloudTransfer,
    quote:Approval, onBoard:()=>void, beforeApproval:()=>Promise<void>) {
  const m=transfer.market;
  if (!m || m.user!==bound.user) throw new Error('LAB_BOARD_ACCOUNT');
  return connect(handle,async (active,{native,owner,registry})=>{
    if(owner.boardFingerprint!==bound.fingerprint || owner.generation!==bound.generation) throw new Error('LAB_BOARD_REENROLLMENT');
    const rpc=httpsEvmRpc(registry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
    const issued=await rpc('nu54_marketBoardGrant',[{token:registry.token,venue:profile.venue.address,genesis:profile.genesis,
      deployment:profile.deploymentHash,user:m.user,to:transfer.to,nonce:transfer.nonce,gasPrice:transfer.maxFeeWei,quote:m.offer,
      allowance:m.offer===m.calldata?'':m.calldata}]);
    const wire=readGrant(issued,transfer,profile,quote.deadline);
    await beforeApproval();
    const signed=await active.signMarket(wire,owner.address,bound.user,BigInt(bound.generation),onBoard);
    return verifyMarketSigned(signed,transfer);
  });
}
