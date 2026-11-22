import {secp256k1} from '@noble/curves/secp256k1';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes, bytesToHex, concat, hexToBytes} from '@nu54/protocol';

import NativeLab from '../specs/NativeMarketLab.ts';
import {cloudMarketProfile, type CloudTransfer} from '../wallet/cloud/client.ts';
export const chainId = 8283;
export const operations = [
  {id: 'swap-base', label: 'LAB → LAQ 교환', group: '교환', fields: [['a', '판매 LAB', 8], ['minA', '최소 수령 LAQ', 6]], out: ['LAQ', 6]},
  {id: 'swap-quote', label: 'LAQ → LAB 교환', group: '교환', fields: [['a', '판매 LAQ', 6], ['minA', '최소 수령 LAB', 8]], out: ['LAB', 8]},
  {id: 'lp-add', label: '유동성 공급', group: '유동성', fields: [['a', '최대 지급 LAB', 8], ['b', '최대 지급 LAQ', 6], ['minA', '최소 발행 지분', 8]], out: ['LP 지분', 8]},
  {id: 'lp-remove', label: '유동성 회수', group: '유동성', fields: [['a', '소각 지분', 8], ['minA', '최소 수령 LAB', 8], ['minB', '최소 수령 LAQ', 6]], out: ['LAB', 8]},
  {id: 'fx-base', label: 'LAB → LAQ 모의 환전', group: '모의 환전', fields: [['a', '가상 매도 LAB', 8], ['minA', '최소 가상 수령 LAQ', 6]], out: ['LAQ', 6]},
  {id: 'fx-quote', label: 'LAQ → LAB 모의 환전', group: '모의 환전', fields: [['a', '가상 매도 LAQ', 6], ['minA', '최소 가상 수령 LAB', 8]], out: ['LAB', 8]},
  {id: 'open-long', label: '매수 포지션 열기', group: '포지션', fields: [['a', '격리 증거금 LAQ', 6], ['b', '포지션 수량 LAB', 8]], out: ['LAQ 증거금', 6]},
  {id: 'open-short', label: '매도 포지션 열기', group: '포지션', fields: [['a', '격리 증거금 LAQ', 6], ['b', '포지션 수량 LAB', 8]], out: ['LAQ 증거금', 6]},
  {id: 'close', label: '포지션 종료', group: '포지션', fields: [['a', '포지션 번호', 0], ['b', '포지션 버전', 0], ['minA', '최소 수령 LAQ', 6]], out: ['LAQ', 6]},
] as const;
export type Operation = typeof operations[number];
export type Draft = Record<'a' | 'b' | 'minA' | 'minB', string>;
export type Intent = Draft & {user: string; action: Operation['id']};
export type Approval = Intent & {
  priceVersion: string; price: string; observedAt: number; deadline: number; first: string; second: string;
  digest: string; to: string; data: string; nonce: string;
};
type Target = {address: string; codeHash: string};
export type Profile = {environment: string; chainId: number; genesis: string; deploymentHeight: number;
  deploymentHash: string; quoteSigner: string; venue: Target; base: Target; quote: Target};
export type Snapshot = {profile: Profile; state: 'ready' | 'paused' | 'price-unavailable' | 'head-clock-unavailable'; time: number; price?: string;
  priceVersion?: string; observations: {price: string; observedAt: string}[]};
export type Wallet = {nonce: number; gasPrice: string; gasBalance: string; baseBalance: string; quoteBalance: string;
  baseAllowance: string; quoteAllowance: string; shares: string};
export type Position = {owner: string; long: boolean; closed: boolean; initialMargin: string; margin: string;
  size: string; entryPrice: string; revision: string; nextPeriod: string};
export type PositionEvent = {kind:'opened'|'funded'|'closed'|'liquidated'; txHash:string; block:number;
  period?:string; priceVersion?:string; payment?:string; margin?:string; payout?:string; fundingGap?:string};
export type PositionView = {position: Position; equity?: string; maintenance?: string};
export type Result = {txHash: string; state: 'pending'|'confirmed'|'rejected'; block?: number; blockHash?: string;
  action?: Operation['id']; gasFee?: string; first?: string; second?: string; addedBase?: string; addedQuote?: string};
export type Checkpoint = {raw: string; hash: string; digest?: string; offer?: string; fundingAt?: number; phase?: 'staged' | 'attempted'};
const max = 10n ** 24n;
export const validAddr = (v: string) => /^0x[0-9a-f]{40}$/i.test(v);
export const validHash = (v: string) => /^0x[0-9a-f]{64}$/i.test(v);
const hex = (bytes: Uint8Array) => bytesToHex(bytes, true);
const unhex = (value: string) => hexToBytes(value.slice(2));
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();
const readWord = (word: Uint8Array) => BigInt(hex(word));
const hashText = (s: string) => keccak_256(Uint8Array.from(s, c => c.charCodeAt(0)));
const word = (x: bigint) => bigToBytes(x, 32);
const bounded = (x: string) => {
  if (!/^(?:0|[1-9][0-9]{0,77})$/.test(x) || BigInt(x) >= 2n ** 256n) throw new Error('LAB_UNITS');
  return BigInt(x);
};

export function decimalUnits(input: string, decimals: number) {
  if (!/^(?:0|[1-9][0-9]{0,24})(?:\.[0-9]{1,8})?$/.test(input)) throw new Error('LAB_DECIMAL');
  const [whole, fraction = ''] = input.split('.');
  if (fraction.length > decimals) throw new Error('LAB_DECIMALS');
  const amount = BigInt(whole + fraction.padEnd(decimals, '0'));
  if (amount > max) throw new Error('LAB_AMOUNT');
  return amount.toString();
}

export function displayAmount(input: string, decimals: number) {
  const value = bounded(input);
  if (decimals === 0) return value.toString();
  const scale = 10n ** BigInt(decimals);
  const tail = (value % scale).toString().padStart(decimals, '0').replace(/0+$/, '');
  return (value / scale).toString() + (tail ? '.' + tail : '');
}

export function validateProfile(profile: Profile) {
  if (!profile || profile.environment !== 'p08-isolated-8283-v1' || profile.chainId !== chainId ||
    !Number.isSafeInteger(profile.deploymentHeight) || profile.deploymentHeight <= 0 ||
    !validHash(profile.genesis) || !validHash(profile.deploymentHash) || !validAddr(profile.quoteSigner)) throw new Error('LAB_PROFILE');
  const targets = [profile.venue, profile.base, profile.quote];
  for (const target of targets) {
    if (!target || !validAddr(target.address) || /^0x0{40}$/i.test(target.address) || !validHash(target.codeHash) ||
        /^0x0{64}$/i.test(target.codeHash)) throw new Error('LAB_TARGET');
  }
  if (new Set(targets.map(target => target.address.toLowerCase())).size !== targets.length) throw new Error('LAB_TARGET');
  return profile;
}

export function verifyDeploymentPolicy(policy: string, profile: Profile) {
  validateProfile(profile);
  const targets = [profile.venue, profile.base, profile.quote].flatMap(target => [target.address, target.codeHash]);
  const digest = hex(hashText([profile.environment, profile.chainId, profile.genesis, profile.deploymentHeight,
    profile.deploymentHash, profile.quoteSigner, ...targets].join('|').toLowerCase()));
  if (policy !== 'loopback' && digest !== policy) throw new Error('LAB_DEPLOYMENT_POLICY');
  return digest;
}

export async function installedMarketPolicy() {
  return await NativeLab?.deploymentPolicy() ?? '';
}

export async function installedMarketOperatorOrigin() {
  const policy = await installedMarketPolicy();
  if (!/^0x[0-9a-f]{64}$/.test(policy)) return '';
  const origin = await NativeLab?.operatorOrigin?.() ?? '';
  const match = /^https:\/\/[a-z0-9][a-z0-9.-]*[.][a-z][a-z0-9-]*(?::([1-9][0-9]{0,4}))?$/.exec(origin);
  return match && match[0] === origin && origin.length <= 260 && Number(match[1] ?? 443) <= 65535 ? origin : '';
}

export function sameDeployment(expected: Profile, actual: Profile) {
  validateProfile(actual);
  return same(expected.genesis, actual.genesis) && same(expected.deploymentHash, actual.deploymentHash) &&
    expected.deploymentHeight === actual.deploymentHeight && same(expected.quoteSigner, actual.quoteSigner) &&
    (['venue', 'base', 'quote'] as const).every(target =>
      same(expected[target].address, actual[target].address) && same(expected[target].codeHash, actual[target].codeHash));
}

export function makeIntent(operation: Operation, draft: Partial<Draft>, user: string): Intent {
  const parsed = {a: '0', b: '0', minA: '0', minB: '0'};
  for (const [field, , decimals] of operation.fields) {
    parsed[field] = decimalUnits(draft[field] ?? '', decimals);
  }
  if (!validAddr(user) || parsed.a === '0') throw new Error('LAB_INTENT');
  return {...parsed, user, action: operation.id};
}

const quoteType = hashText('Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)');
const domainType = hashText('EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)');
const executeSelector = hex(hashText('execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)').slice(0,4));

export function verifyApproval(p: Profile, intent: Intent, q: Approval, chainTime: number) {
  if (!q || !same(q.to, p.venue.address) || !same(q.user, intent.user) || q.action !== intent.action ||
      q.a !== intent.a || q.b !== intent.b || q.minA !== intent.minA || q.minB !== intent.minB ||
      !Number.isSafeInteger(q.deadline) || !Number.isSafeInteger(q.observedAt) || !Number.isSafeInteger(chainTime) ||
      q.deadline <= chainTime || q.deadline > chainTime + 60 || q.observedAt > chainTime ||
      q.observedAt + 120 < q.deadline || !/^0x[0-9a-f]{904}$/i.test(q.data) || !validHash(q.digest)) {
    throw new Error('LAB_QUOTE');
  }
  const calldata = unhex(q.data);
  const actionIndex = operations.findIndex(op => op.id === intent.action);
  if (actionIndex < 0 || hex(calldata.slice(0,4)) !== executeSelector) throw new Error('LAB_SELECTOR');
  const terms = [BigInt(intent.user), BigInt(actionIndex), bounded(intent.a), bounded(intent.b), bounded(intent.minA),
    bounded(intent.minB), bounded(q.priceVersion), bounded(q.nonce), BigInt(q.deadline)];
  const encoded = concat(...terms.map(word));
  if (hex(calldata.slice(4,292)) !== hex(encoded) || readWord(calldata.slice(292,324)) !== 320n ||
      readWord(calldata.slice(324,356)) !== 65n) throw new Error('LAB_TERMS');
  const domain = keccak_256(concat(domainType, hashText('NU54 Isolated Market'), hashText('1'), word(BigInt(chainId)),
    word(BigInt(p.venue.address)), unhex(p.venue.codeHash)));
  const message = keccak_256(concat(quoteType, encoded));
  const digest = keccak_256(concat(Uint8Array.of(25,1), domain, message));
  if (!same(hex(digest), q.digest)) throw new Error('LAB_DIGEST');
  const recovery = calldata[420] - 27;
  if (recovery < 0 || recovery > 1 || calldata.slice(421).some(value => value !== 0)) throw new Error('LAB_SIGNATURE');
  const signature = secp256k1.Signature.fromCompact(calldata.slice(356,420)).addRecoveryBit(recovery);
  const recovered = hex(keccak_256(signature.recoverPublicKey(digest).toRawBytes(false).slice(1)).slice(12));
  if (signature.hasHighS() || !same(recovered,p.quoteSigner)) throw new Error('LAB_SIGNATURE');
  if (bounded(q.price) <= 0n || bounded(q.first) < bounded(q.minA) || bounded(q.second) < bounded(q.minB))
    throw new Error('LAB_PREVIEW');
  return q;
}

function scope(p: Profile) {
  return [p.venue.address,p.base.address,p.quote.address,p.quoteSigner,p.venue.codeHash]
    .map(value => value.slice(2).toLowerCase()).join('');
}

export async function isolatedWallet(p: Profile, create: boolean) {
  if (!NativeLab) throw new Error('LAB_UNAVAILABLE');
  validateProfile(p);
  const user = await NativeLab.walletAddress(scope(p),create);
  if (!validAddr(user)) throw new Error('LAB_WALLET');
  return user.toLowerCase();
}

export async function deleteIsolatedWallet(p: Profile) {
  if (!NativeLab || !await NativeLab.deleteWallet(scope(p))) throw new Error('LAB_DELETE');
}

async function signed(p: Profile, wallet: Wallet, target: string, calldata: string, quote: string): Promise<Checkpoint> {
  if (!NativeLab || p.chainId !== chainId || !validAddr(target) || !/^0x(?:[0-9a-f]{2})+$/i.test(calldata) ||
      !Number.isSafeInteger(wallet.nonce) || wallet.nonce < 0) throw new Error('LAB_TX_PROFILE');
  const gas = target === p.venue.address ? 600000n : 65000n;
  const price = bounded(wallet.gasPrice);
  if (price === 0n || price > 100_000_000_000n || bounded(wallet.gasBalance) < gas * price) throw new Error('LAB_GAS');
  const raw = await NativeLab.sign(scope(p),target.slice(2),wallet.nonce,Number(price),calldata.slice(2),quote.slice(2));
  if (!/^0x(?:[a-f0-9]{2}){1,600}$/.test(raw)) throw new Error('LAB_NATIVE_SIGNATURE');
  return {raw,hash:hex(keccak_256(unhex(raw)))};
}

export function requiredAllowance(q: Intent, wallet: Wallet): {token: 'base'|'quote'; amount: string} | undefined {
  if ((q.action === 'lp-add' || q.action === 'swap-base') && bounded(q.a) > bounded(wallet.baseAllowance)) {
    return {token:'base',amount:q.a};
  }
  const amount = q.action === 'lp-add' ? q.b : q.a;
  if (['lp-add','swap-quote','open-long','open-short'].includes(q.action) && bounded(amount) > bounded(wallet.quoteAllowance)) {
    return {token:'quote',amount};
  }
}

export function verifyHoldings(intent: Intent, wallet: Wallet) {
  const safe = (amount: string, available: string) => bounded(amount) <= bounded(available);
  if (
    (['swap-base','lp-add'].includes(intent.action) && !safe(intent.a,wallet.baseBalance)) ||
    (['swap-quote','open-long','open-short'].includes(intent.action) && !safe(intent.a,wallet.quoteBalance)) ||
    (intent.action === 'lp-add' && !safe(intent.b,wallet.quoteBalance)) ||
    (intent.action === 'lp-remove' && !safe(intent.a,wallet.shares))
  ) throw new Error('LAB_BALANCES');
}

function boundAllowance(p: Profile, allowance: string, offerDigest: string) {
  return hex(concat(unhex('0x095ea7b3'),word(BigInt(p.venue.address)),word(bounded(allowance)),unhex(offerDigest)));
}

export function prepareCloudMarketTransfer(p:Profile, user:string, wallet:Wallet, intent:Intent, quote:Approval,
  now:number, allowance:boolean): CloudTransfer {
  verifyApproval(p,intent,quote,now);
  verifyHoldings(intent,wallet);
  const needed=requiredAllowance(intent,wallet);
  if (!same(user,intent.user) || allowance!==!!needed || !Number.isSafeInteger(wallet.nonce) || wallet.nonce<0)
    throw new Error('LAB_CLOUD_ORDER');
  const target=needed ? p[needed.token].address : p.venue.address;
  const calldata=needed
    ? boundAllowance(p,needed.amount,quote.digest) : quote.data;
  const gas=needed?65000n:600000n;
  const price=bounded(wallet.gasPrice);
  if(price===0n || price>100_000_000_000n || bounded(wallet.gasBalance)<gas*price) throw new Error('LAB_GAS');
  return {profile:cloudMarketProfile, chain:'8283',to:target.toLowerCase(),valueWei:'0',nonce:String(wallet.nonce),
    maxFeeWei:wallet.gasPrice,priorityFeeWei:'0',market:{deploymentHash:p.deploymentHash.toLowerCase(),
      user:user.toLowerCase(),calldata:calldata.toLowerCase(),offer:quote.data.toLowerCase()}};
}

export function signedCloudCheckpoint(raw:string, digest?:string, offer?:string):Checkpoint {
  if (!/^0x(?:[a-f0-9]{2}){1,600}$/.test(raw) || (digest!==undefined && !validHash(digest))) throw new Error('LAB_CLOUD_SIGNATURE');
  return verifyCheckpoint({raw,hash:hex(keccak_256(unhex(raw))),digest,offer});
}

export async function signAllowance(p: Profile, user: string, wallet: Wallet, intent: Intent, quote: Approval, now: number) {
  verifyApproval(p,intent,quote,now);
  verifyHoldings(intent,wallet);
  const needed = requiredAllowance(intent,wallet);
  if (!needed || !same(intent.user,user)) throw new Error('LAB_ALLOWANCE');
  const data = boundAllowance(p,needed.amount,quote.digest);
  return {...await signed(p,wallet,p[needed.token].address,data,quote.data),offer:quote.data};
}

export async function signExecution(p: Profile, user: string, wallet: Wallet, intent: Intent, q: Approval, now: number) {
  if (!same(intent.user,user) || requiredAllowance(intent,wallet)) throw new Error('LAB_WALLET');
  verifyApproval(p,intent,q,now);
  verifyHoldings(intent,wallet);
  return {...await signed(p,wallet,p.venue.address,q.data,q.data),digest:q.digest,offer:q.data};
}

export function verifyCheckpoint(checkpoint: Checkpoint) {
  if (!checkpoint || !/^0x(?:[a-f0-9]{2}){1,2048}$/.test(checkpoint.raw) ||
      !validHash(checkpoint.hash) || hex(keccak_256(unhex(checkpoint.raw))) !== checkpoint.hash ||
      (checkpoint.digest !== undefined && !validHash(checkpoint.digest)) ||
      (checkpoint.offer !== undefined && !/^0x[0-9a-f]{904}$/i.test(checkpoint.offer)) ||
      (checkpoint.fundingAt !== undefined && (!Number.isSafeInteger(checkpoint.fundingAt) || checkpoint.fundingAt <= 0)) ||
      (checkpoint.phase !== undefined && (checkpoint.offer === undefined || !['staged','attempted'].includes(checkpoint.phase)))) throw new Error('LAB_CHECKPOINT');
  return checkpoint;
}

export async function marketAPI<T>(path: string, body?: unknown): Promise<T> {
  if (!NativeLab) throw new Error('LAB_UNAVAILABLE');
  const reply = await NativeLab.exchange(path,body === undefined ? '' : JSON.stringify(body));
  return JSON.parse(reply) as T;
}
