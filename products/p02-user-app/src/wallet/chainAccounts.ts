import {checksumDescriptor} from './bitcoinDescriptor.ts';
import {hmac} from '@noble/hashes/hmac';
import {sha512} from '@noble/hashes/sha512';
import {secp256k1} from '@noble/curves/secp256k1';
import {ripemd160} from '@noble/hashes/ripemd160';
import {sha256} from '@noble/hashes/sha256';
import {bytesToBig, bytesToHex, hexToBytes, equalBytes} from '@nu54/protocol';

export const bitcoinSignetCheckpoint = {height: 321295,
  hash: '00000002a5e0ba0498f1e9f4591af0b66b63c654665efe65206fd0ae7bbaf923'} as const;

export type WalletChain = 1 | 2 | 3;
export type WalletNetwork = 0 | 1 | 2;
export type ChainAccount = {chain: WalletChain; network: WalletNetwork; account: number};

const bitcoinAlphabet = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
const rippleAlphabet = 'rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz';
const bech32Alphabet = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';

export const chainLabels: Record<WalletChain, string> = {1: 'Bitcoin', 2: 'Solana', 3: 'XRP Ledger'};

export function base58(data: Uint8Array, chars = bitcoinAlphabet): string {
  let value = 0n;
  let result = '';
  for (const byte of data) value = value * 256n + BigInt(byte);
  while (value) {
    result = chars[Number(value % 58n)] + result;
    value /= 58n;
  }
  for (const byte of data) {
    if (byte) break;
    result = chars[0] + result;
  }
  return result;
}

export const doubleSha256 = (bytes: Uint8Array) => sha256(sha256(bytes));
export const hash160 = (bytes: Uint8Array) => ripemd160(sha256(bytes));

export function classicXrpl(account: Uint8Array): string {
  if (account.length !== 20) throw new Error('XRPL_ACCOUNT_ID');
  const input = Uint8Array.of(0, ...account);
  return base58(Uint8Array.of(...input, ...doubleSha256(input).slice(0, 4)), rippleAlphabet);
}

export function bitcoinHrp(network: WalletNetwork) {return network === 2 ? 'tb' : network ? 'bcrt' : 'bc';}

export function segwitAddress(hrp: string, script: Uint8Array): string {
  if (script[0] !== 0 || ![20, 32].includes(script[1]) || script.length !== script[1] + 2)
    throw new Error('BTC_WITNESS_PROGRAM');
  const program = [0];
  let word = 0, bits = 0;
  for (const byte of script.subarray(2)) {
    word = (word << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      program.push((word >> bits) & 31);
    }
  }
  if (bits) program.push((word << (5 - bits)) & 31);
  const letters = Array.from(hrp, char => char.charCodeAt(0));
  const generators = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
  let sum = 1;
  for (const item of [...letters.map(n => n >> 5), 0, ...letters.map(n => n & 31), ...program, 0, 0, 0, 0, 0, 0]) {
    const top = sum >>> 25;
    sum = ((sum & 0x1ffffff) << 5) ^ item;
    generators.forEach((generator, bit) => {
      if ((top >>> bit) & 1) sum ^= generator;
    });
  }
  sum ^= 1;
  const checksum = Array.from({length: 6}, (_, index) => (sum >>> (5 * (5 - index))) & 31);
  return hrp + '1' + [...program, ...checksum].map(value => bech32Alphabet[value]).join('');
}

export function chainAccountCommand(op: 10 | 11, value: ChainAccount): Uint8Array {
  const {chain, network, account} = value;
  if (![1, 2, 3].includes(chain) || ![0, 1, 2].includes(network) ||
      !Number.isInteger(account) || account < 0 || account > 99) throw new Error('CHAIN_ACCOUNT_PROFILE');
  return Uint8Array.of(op, chain, network, 0, 0, 0, account);
}

export type ChainPublicAccount = ChainAccount & {address: string; publicKey: Uint8Array; fingerprint?: string};

export function chainPublicCommand(profile: ChainAccount, branch: 0 | 1, index = 0): Uint8Array {
  if ((branch || index) && profile.chain !== 1) throw new Error('CHAIN_CHANGE_BRANCH');
  if (!Number.isInteger(index) || index < 0 || index > 9999) throw new Error('CHAIN_ADDRESS_INDEX');
  const head = chainAccountCommand(10, profile);
  return index ? Uint8Array.of(...head, 0, 0, 0, branch, 0, 0, index >>> 8, index & 255) :
    Uint8Array.of(...head, 0, 0, 0, branch);
}

export function readChainPublic(raw: Uint8Array, profile: ChainAccount, branch: 0 | 1, index = 0): ChainPublicAccount {
  const header = chainPublicCommand(profile, branch, index);
  const offset = header.length;
  if (raw.length !== offset + 57 || !equalBytes(raw.subarray(0, offset), header))
    throw new Error('CHAIN_ACCOUNT_REPLY');
  if (profile.chain === 2) {
    const publicKey = raw.slice(offset, offset + 32);
    if (raw.subarray(offset + 32).some(Boolean) || !publicKey.some(Boolean)) throw new Error('SOL_PUBLIC_KEY');
    return {...profile, publicKey, address: base58(publicKey)};
  }
  const publicKey = raw.slice(offset, offset + 33);
  secp256k1.ProjectivePoint.fromHex(publicKey);
  const keyHash = hash160(publicKey);
  if (!equalBytes(raw.subarray(offset + 33, offset + 53), keyHash)) throw new Error('CHAIN_ACCOUNT_HASH160');
  const address = profile.chain === 1 ?
    segwitAddress(bitcoinHrp(profile.network), Uint8Array.of(0, 20, ...keyHash)) : classicXrpl(keyHash);
  if (profile.chain !== 1 && raw.subarray(offset + 53).some(Boolean)) throw new Error('CHAIN_FINGERPRINT');
  return {...profile, address, publicKey,
    fingerprint: profile.chain === 1 ? bytesToHex(raw.subarray(offset + 53), false) : undefined};
}

export function receiveCard(value: ChainPublicAccount, rawTag: string) {
  if (rawTag && (value.chain !== 3 || !/^(0|[1-9][0-9]{0,9})$/.test(rawTag) ||
      BigInt(rawTag) > 0xffffffffn)) throw new Error('XRPL_DESTINATION_TAG');
  const tag = rawTag || undefined;
  const network = value.network === 2 ? 'commissioned test network' : value.network ? 'local laboratory' : 'mainnet';
  const {address} = value;
  const share = [chainLabels[value.chain] + ' · ' + network, 'Address: ' + address,
    ...(tag !== undefined ? ['DestinationTag: ' + tag] : [])].join('\n');
  // Solana and the classic XRP address do not identify their network.
  // Include the network in the QR to avoid a live-wallet URI for a lab address.
  const uri = value.chain === 1 ? 'bitcoin:' + address : share;
  return {uri, address, tag, network, share};
}

export function bitcoinXpubCommand(profile: ChainAccount): Uint8Array {
  if (profile.chain !== 1 || ![1,2].includes(profile.network)) throw new Error('BTC_XPUB_PROFILE');
  const bytes = chainAccountCommand(10, profile);
  bytes[0] = 13;
  return bytes;
}

export type StoredBitcoinWatch = {account:ChainAccount; board:string; generation:number; proof:string};

export function readStoredBitcoinWatch(input:unknown):input is StoredBitcoinWatch {
  if (!input || typeof input!=='object') return false;
  const value=input as StoredBitcoinWatch;
  if (Object.keys(value).sort().join(',') !== 'account,board,generation,proof' ||
      !value.account || Object.keys(value.account).sort().join(',') !== 'account,chain,network' ||
      !/^[a-f0-9]{64}$/.test(value.board) || !Number.isSafeInteger(value.generation) || value.generation<1 ||
      !/^[a-f0-9]{160}$/.test(value.proof)) return false;
  try {bitcoinWatchDescriptors(hexToBytes(value.proof),value.account);return true;} catch {return false;}
}

export function bitcoinWatchDescriptors(raw: Uint8Array, profile: ChainAccount) {
  if (raw.length === 1 && raw[0] === 0) throw new Error('CHAIN_USER_REFUSED');
  if (raw.length !== 80 || !equalBytes(raw.subarray(0, 7), bitcoinXpubCommand(profile)) ||
      !raw.subarray(15, 47).some(Boolean)) throw new Error('BTC_XPUB_REPLY');
  secp256k1.ProjectivePoint.fromHex(raw.subarray(47));
  // Base58Check BIP32 tpub. Script type is encoded in the output descriptor.
  const serialized = Uint8Array.of(0x04, 0x35, 0x87, 0xcf, 3, ...raw.subarray(11, 15),
    0x80, 0, 0, profile.account, ...raw.subarray(15));
  const xpub = base58(Uint8Array.of(...serialized, ...doubleSha256(serialized).subarray(0, 4)));
  const masterFingerprint = bytesToHex(raw.subarray(7, 11), false);
  const setup = '[' + masterFingerprint + '/84h/1h/' + profile.account + 'h]' + xpub;
  const publicChild = (branch:0|1,addressIndex:number) => {
    if ((branch!==0 && branch!==1) || !Number.isInteger(addressIndex) || addressIndex<0 || addressIndex>9999)
      throw new Error('BTC_XPUB_CHILD');
    let publicKey: Uint8Array = raw.slice(47);
    let chainCode = raw.slice(15, 47);
    for (const index of [branch,addressIndex]) {
      const digest = hmac(sha512,chainCode,Uint8Array.of(...publicKey,0,0,Math.floor(index/256),index%256));
      const scalar = bytesToBig(digest.subarray(0, 32));
      const point = secp256k1.ProjectivePoint.fromHex(publicKey);
      if (!scalar || scalar >= secp256k1.CURVE.n) throw new Error('BTC_XPUB_CHILD');
      const child = point.add(secp256k1.ProjectivePoint.BASE.multiply(scalar));
      if (child.equals(secp256k1.ProjectivePoint.ZERO)) throw new Error('BTC_XPUB_CHILD');
      publicKey = child.toRawBytes(true);
      chainCode = digest.slice(32);
    }
    return publicKey;
  };
  const firstChild=(branch:0|1)=>segwitAddress(bitcoinHrp(profile.network),Uint8Array.of(0,20,...hash160(publicChild(branch,0))));
  return {xpub, masterFingerprint, receive: checksumDescriptor('wpkh(' + setup + '/0/*)'),
    change: checksumDescriptor('wpkh(' + setup + '/1/*)'), firstReceive: firstChild(0), firstChange: firstChild(1),publicChild};
}


export function bip48Participant(raw: Uint8Array, profile: ChainAccount) {
  if (raw.length === 1 && raw[0] === 0) throw new Error('CHAIN_USER_REFUSED');
  const command = Uint8Array.of(...bitcoinXpubCommand(profile), 48);
  if (raw.length !== 81 || !equalBytes(raw.subarray(0, 8), command) || !raw.subarray(16, 48).some(Boolean))
    throw new Error('BIP48_PARTICIPANT');
  secp256k1.ProjectivePoint.fromHex(raw.subarray(48));
  const serialized = Uint8Array.of(0x04, 0x35, 0x87, 0xcf, 4, ...raw.subarray(12, 16),
    0x80, 0, 0, 2, ...raw.subarray(16));
  const xpub = base58(Uint8Array.of(...serialized, ...doubleSha256(serialized).subarray(0, 4)));
  return '[' + bytesToHex(raw.subarray(8, 12), false) + '/48h/1h/' + profile.account + 'h/2h]' + xpub;
}

export function decodePublicBase58(value: string, length: number, chars = bitcoinAlphabet): Uint8Array {
  if (value.length < 1 || value.length > 120) throw new Error('CHAIN_ADDRESS_ENCODING');
  let n = 0n;
  for (const char of value) {
    const digit = chars.indexOf(char);
    if (digit < 0) throw new Error('CHAIN_ADDRESS_ENCODING');
    n = n * 58n + BigInt(digit);
  }
  const output = new Uint8Array(length);
  for (let i = length - 1; i >= 0; i--) {
    output[i] = Number(n & 255n);
    n >>= 8n;
  }
  if (n || base58(output, chars) !== value) throw new Error('CHAIN_ADDRESS_ENCODING');
  return output;
}

export function checkXrplClassic(address: string) {
  const decoded = decodePublicBase58(address, 25, rippleAlphabet);
  if (decoded[0] !== 0 || classicXrpl(decoded.subarray(1, 21)) !== address)
    throw new Error('XRPL_ADDRESS_CHECKSUM');
  return address;
}

export function xrplXAddress(address: string, tag: string | undefined, test: boolean) {
  checkXrplClassic(address);
  if (tag !== undefined && (!/^(0|[1-9][0-9]{0,9})$/.test(tag) || BigInt(tag) > 0xffffffffn))
    throw new Error('XRPL_DESTINATION_TAG');
  const account = decodePublicBase58(address, 25, rippleAlphabet).subarray(1, 21);
  const encoded = Uint8Array.of(...(test ? [4, 147] : [5, 68]), ...account, tag === undefined ? 0 : 1,
    0, 0, 0, 0, 0, 0, 0, 0);
  if (tag !== undefined) new DataView(encoded.buffer).setUint32(23, Number(tag), true);
  return base58(Uint8Array.of(...encoded, ...doubleSha256(encoded).subarray(0, 4)), rippleAlphabet);
}

export function normalizeXrplDestination(address: string, tag: number | undefined, test: boolean) {
  if (tag !== undefined && (!Number.isInteger(tag) || tag < 0 || tag > 0xffffffff))
    throw new Error('XRPL_DESTINATION_TAG');
  if (address.startsWith('r')) {
    if (address===classicXrpl(new Uint8Array(20))) throw new Error('XRPL_ADDRESS_ENCODING');
    return {destination:checkXrplClassic(address), tag};
  }
  const raw = decodePublicBase58(address, 35, rippleAlphabet);
  const [first, second] = test ? [4, 147] : [5, 68];
  if (raw[0] !== first || raw[1] !== second) throw new Error('XRPL_ADDRESS_NETWORK');
  if (raw[22] > 1 || raw.subarray(raw[22] ? 27 : 23,31).some(Boolean))
    throw new Error('XRPL_DESTINATION_TAG');
  const embedded = raw[22] ? new DataView(raw.buffer).getUint32(23,true) : undefined;
  if (!raw.subarray(2,22).some(Boolean)) throw new Error('XRPL_ADDRESS_ENCODING');
  const destination = classicXrpl(raw.subarray(2,22));
  if (xrplXAddress(destination,embedded?.toString(),test) !== address) throw new Error('XRPL_ADDRESS_CHECKSUM');
  if (tag !== undefined && embedded !== undefined && tag !== embedded) throw new Error('XRPL_DESTINATION_TAG');
  return {destination,tag:embedded ?? tag};
}

export function checkSegwitV0(hrp: 'bc' | 'tb' | 'bcrt', address: string) {
  const canonical = address.toLowerCase();
  if ((canonical !== address && address.toUpperCase() !== address) ||
      !canonical.startsWith(hrp + '1q') || canonical.length > 90) throw new Error('BTC_ADDRESS_NETWORK');
  const words = canonical.slice(hrp.length + 2, -6);
  let bits = 0, accumulator = 0;
  const program: number[] = [];
  for (const char of words) {
    const word = bech32Alphabet.indexOf(char);
    if (word < 0) throw new Error('BTC_ADDRESS_ENCODING');
    accumulator = (accumulator << 5) | word;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      program.push((accumulator >>> bits) & 255);
    }
  }
  if (![20, 32].includes(program.length) ||
      segwitAddress(hrp, Uint8Array.of(0, program.length, ...program)) !== canonical)
    throw new Error('BTC_ADDRESS_CHECKSUM');
  return canonical;
}
