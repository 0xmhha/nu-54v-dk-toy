import {ed25519} from '@noble/curves/ed25519';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex, hexToBytes, utf8Encode} from '@nu54/protocol';

const hex64 = /^[0-9a-f]{64}$/;
export type InstallExpectation = {publicKey:string; channel:'official'|'custom'; candidate:string; device:string;
  bootKey:string; measurement:string; minimumCounter:number; generation:number; challenge:string; now:number};
export type InstallClaim = {format:'nu54-board-install-v1'; model:string; channel:'official'|'custom'; candidate:string;
  device:string; bootKey:string; measurement:string; counter:number; generation:number; challenge:string;
  issuedAt:number; expiresAt:number; evidence:'observed'};
/** Signed observed installation evidence is not a substitute for authenticated board enrollment. */
export function verifyInstallation(envelope:{payload:string;signature:string}, expected:InstallExpectation):InstallClaim {
  if (!envelope || typeof envelope.payload !== 'string' || envelope.payload.length > 4096 ||
      !/^[0-9a-f]{128}$/.test(envelope.signature) || !hex64.test(expected.publicKey) ||
      !['custom','official'].includes(expected.channel) || !Number.isSafeInteger(expected.minimumCounter) || expected.minimumCounter < 1 ||
      !Number.isSafeInteger(expected.generation) || expected.generation < 0 || !Number.isSafeInteger(expected.now))
    throw new Error('INSTALL_RECEIPT_INVALID');
  let item:InstallClaim;
  try { item=JSON.parse(envelope.payload); } catch { throw new Error('INSTALL_RECEIPT_INVALID'); }
  if (!item || JSON.stringify(item) !== envelope.payload ||
      Object.keys(item).sort().join(',') !== ['format','model','channel','candidate','device','bootKey','measurement','counter','generation','challenge','issuedAt','expiresAt','evidence'].sort().join(',') ||
      item.format !== 'nu54-board-install-v1' || item.model !== 'nu54v_dk/nrf54l15/cpuapp' || item.evidence !== 'observed' ||
      item.channel !== expected.channel || item.candidate !== expected.candidate || item.device !== expected.device ||
      item.bootKey !== expected.bootKey || item.measurement !== expected.measurement || item.challenge !== expected.challenge || item.generation !== expected.generation ||
      ![item.candidate,item.device,item.bootKey,item.measurement,item.challenge].every(v=>typeof v === 'string' && hex64.test(v) && v !== '0'.repeat(64)) ||
      !Number.isSafeInteger(item.counter) || item.counter < expected.minimumCounter || !Number.isSafeInteger(item.generation) || item.generation < 0 ||
      !Number.isSafeInteger(item.issuedAt) || !Number.isSafeInteger(item.expiresAt) || item.issuedAt > expected.now ||
      item.expiresAt <= expected.now || item.expiresAt <= item.issuedAt || item.expiresAt - item.issuedAt > 300 ||
      !ed25519.verify(hexToBytes(envelope.signature), utf8Encode(envelope.payload), hexToBytes(expected.publicKey), {zip215:false})) {
    throw new Error('INSTALL_RECEIPT_INVALID');
  }
  return item;
}

export type InstallClock = {boot:string; uptimeMs:number; wallMs:number};
export function readInstallClock(raw:string):InstallClock {
  let c:InstallClock;
  try { c=JSON.parse(raw); } catch { throw new Error('INSTALL_CLOCK_INVALID'); }
  if (!c || Object.keys(c).sort().join(',') !== 'boot,uptimeMs,wallMs' ||
      typeof c.boot !== 'string' || !/^(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|android-boot-[0-9]{1,10})$/.test(c.boot) ||
      !Number.isSafeInteger(c.uptimeMs) || c.uptimeMs < 0 || !Number.isSafeInteger(c.wallMs) || c.wallMs < 0)
    throw new Error('INSTALL_CLOCK_INVALID');
  return c;
}
/** Fresh OS monotonic time and an unchanged OS boot identifier are mandatory. */
export function installationTime(start:InstallClock, current:InstallClock):number {
  const a=readInstallClock(JSON.stringify(start)), b=readInstallClock(JSON.stringify(current));
  const elapsed=b.uptimeMs-a.uptimeMs;
  if (a.boot !== b.boot || elapsed < 0 || elapsed > 300000 || Math.abs((b.wallMs-a.wallMs)-elapsed)>5000)
    throw new Error('INSTALL_CLOCK_CHANGED');
  return Math.floor((a.wallMs+elapsed)/1000);
}
export type InstallCheckpoint = {state:'pending'; expected:InstallExpectation; clock:InstallClock} |
  {state:'consumed'; expected:InstallExpectation; clock:InstallClock; claim:InstallClaim; receiptDigest:string; envelope:{payload:string;signature:string}};
/** Caller persists the returned checkpoint atomically in protected storage before advancing. */
export function consumeInstallation(checkpoint:InstallCheckpoint, envelope:{payload:string;signature:string}, current:InstallClock):InstallCheckpoint {
  const digest = bytesToHex(sha256(utf8Encode(JSON.stringify(envelope))),false);
  const now=installationTime(checkpoint.clock,current);
  if (checkpoint.state === 'consumed') {
    if (now >= checkpoint.claim.expiresAt) throw new Error('INSTALL_RECEIPT_INVALID');
    if (checkpoint.receiptDigest !== digest) throw new Error('INSTALL_RECEIPT_REPLAY');
    return checkpoint;
  }
  const claim=verifyInstallation(envelope,{...checkpoint.expected,now});
  if (now < checkpoint.expected.now || claim.issuedAt < checkpoint.expected.now || now - checkpoint.expected.now > 300)
    throw new Error('INSTALL_RECEIPT_INVALID');
  return {state:'consumed',expected:checkpoint.expected,clock:checkpoint.clock,claim,receiptDigest:digest,envelope};
}

/** Revalidate the signed receipt after a protected-store restore; it is historical, not a new authorization. */
export function validInstallCheckpoint(value:unknown):value is InstallCheckpoint {
  try {
    if (!value || typeof value !== 'object') return false;
    const item=value as InstallCheckpoint;
    if (!item.expected || !item.clock || !['pending','consumed'].includes(item.state)) return false;
    const c=readInstallClock(JSON.stringify(item.clock));
    const e=item.expected;
    if (Object.keys(e).sort().join(',') !== ['publicKey','channel','candidate','device','bootKey','measurement','minimumCounter','generation','challenge','now'].sort().join(',') ||
        !['custom','official'].includes(e.channel) || ![e.publicKey,e.candidate,e.device,e.bootKey,e.measurement,e.challenge].every(v=>typeof v==='string' && hex64.test(v) && v !== '0'.repeat(64)) ||
        !Number.isSafeInteger(e.minimumCounter) || e.minimumCounter < 1 || !Number.isSafeInteger(e.generation) || e.generation < 0 || !Number.isSafeInteger(e.now) || e.now !== Math.floor(c.wallMs/1000)) return false;
    if (item.state === 'pending') return Object.keys(item).sort().join(',')==='clock,expected,state';
    if (Object.keys(item).sort().join(',')!=='claim,clock,envelope,expected,receiptDigest,state') return false;
    const claim=verifyInstallation(item.envelope,{...e,now:item.claim.issuedAt});
    if (claim.issuedAt < e.now || claim.issuedAt - e.now > 300) return false;
    return JSON.stringify(claim)===JSON.stringify(item.claim) && item.receiptDigest===bytesToHex(sha256(utf8Encode(JSON.stringify(item.envelope))),false);
  } catch {return false;}
}

export type InstallPreflight = {channel:'official'|'custom';candidate:string;bootKey:string;measurement:string;
  minimumCounter:number;publicKey:string};
/** Out-of-band key and candidate verification is mandatory before issuing a new challenge. */
export function startInstallation(preflight:InstallPreflight, independentSigner:string, independentCandidate:string,
  independentDevice:string, generation:number, challenge:string, clock:InstallClock):InstallCheckpoint {
  if (!preflight || preflight.channel !== 'custom' || Object.keys(preflight).sort().join(',') !== ['channel','candidate','bootKey','measurement','minimumCounter','publicKey'].sort().join(',') ||
      !hex64.test(preflight.publicKey) || !hex64.test(independentSigner) || !hex64.test(independentCandidate) ||
      bytesToHex(sha256(hexToBytes('302a300506032b6570032100'+preflight.publicKey)),false) !== independentSigner ||
      preflight.candidate !== independentCandidate) throw new Error('INSTALL_TRUST_MISMATCH');
  const trustedClock=readInstallClock(JSON.stringify(clock));
  const expected:InstallExpectation={...preflight,device:independentDevice,generation,challenge,now:Math.floor(trustedClock.wallMs/1000)};
  const checkpoint:InstallCheckpoint={state:'pending',expected,clock:trustedClock};
  if (!validInstallCheckpoint(checkpoint)) throw new Error('INSTALL_TRUST_MISMATCH');
  return checkpoint;
}

/** Independent trust document is deliberately separate from the release setup file. */
export function readInstallTrust(raw:string):{signer:string;candidate:string;device:string} {
  if (typeof raw !== 'string' || raw.length > 8192) throw new Error('INSTALL_TRUST_MISMATCH');
  let value:{signer:string;candidate:string;device:string};
  try {value=JSON.parse(raw);} catch {throw new Error('INSTALL_TRUST_MISMATCH');}
  if (!value || Object.keys(value).sort().join(',') !== 'candidate,device,signer' ||
      ![value.signer,value.candidate,value.device].every(v=>typeof v==='string' && hex64.test(v) && v !== '0'.repeat(64)))
    throw new Error('INSTALL_TRUST_MISMATCH');
  return value;
}
