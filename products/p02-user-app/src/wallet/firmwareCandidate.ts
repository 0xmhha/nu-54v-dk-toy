import {ed25519} from '@noble/curves/ed25519';
import {bytesToHex,hexToBytes,utf8Encode} from '@nu54/protocol';
import {sha256} from '@noble/hashes/sha256';
import type {RunningFirmware} from './firmwareStatusWire.ts';
import type {InstallClaim} from './boardInstallation.ts';
import type {OwnerStatus} from './ownerWire.ts';

const model='nu54v_dk/nrf54l15/cpuapp';
const digest=/^[a-f0-9]{64}$/;
const revision=/^[a-z0-9][a-z0-9_-]{0,31}$/;

export type FirmwareCandidateTrust = {
  board:string;
  boardRevision:string;
  channel:'custom';
  stationKey:string;
  publisherKey:string;
  bootKey:string;
};

export type FirmwareCandidate = {version:string;counter:number;channel:'custom';issuedAt:number;expiresAt:number};

function firmwareBootPin(key:string) {
  return bytesToHex(sha256(hexToBytes('302a300506032b6570032100'+key)),false);
}

export function candidateTrustForInstallation(trust:FirmwareCandidateTrust|undefined,
  installation:Pick<InstallClaim,'device'|'channel'|'bootKey'>|undefined):FirmwareCandidateTrust|undefined {
  if(!trust||trust.channel!=='custom')return undefined;
  if(!installation||installation.device!==trust.board)return trust;
  return installation.channel===trust.channel&&digest.test(trust.bootKey)&&
    installation.bootKey===firmwareBootPin(trust.bootKey)?trust:undefined;
}

function record(value:unknown,fields:string):Record<string,unknown> {
  if (!value||typeof value!=='object'||Object.keys(value).sort().join(',')!==fields)
    throw new Error('FIRMWARE_CANDIDATE_INVALID');
  return value as Record<string,unknown>;
}

function signedPayload(payload:unknown,signature:unknown,key:string):string {
  if (typeof payload!=='string'||payload.length>4096||typeof signature!=='string'||
      !/^[a-f0-9]{128}$/.test(signature)||!ed25519.verify(hexToBytes(signature),utf8Encode(payload),hexToBytes(key),{zip215:false}))
    throw new Error('FIRMWARE_CANDIDATE_INVALID');
  return payload;
}

export function verifyFirmwareCandidate(raw:string,trust:FirmwareCandidateTrust,board:OwnerStatus,running:RunningFirmware,now:number):FirmwareCandidate {
  if (raw.length>8192||board.state!=='active'||board.device!==trust.board||board.generation<=0n||
      trust.channel!=='custom'||!revision.test(trust.boardRevision)||!Number.isSafeInteger(now)||
      ![trust.stationKey,trust.publisherKey,trust.bootKey].every(key=>digest.test(key)&&!/^[0]+$/.test(key))||
      new Set([trust.stationKey,trust.publisherKey,trust.bootKey]).size!==3)
    throw new Error('FIRMWARE_CANDIDATE_SCOPE');
  const envelope=record(JSON.parse(raw),'payload,signature');
  const claim=record(JSON.parse(signedPayload(envelope.payload,envelope.signature,trust.stationKey)),
    'board,bootKey,counter,descriptor,descriptorSignature,expiresAt,format,generation,issuedAt,measurement,model');
  const {issuedAt,expiresAt}=claim;
  const bootPin=firmwareBootPin(trust.bootKey);
  if(claim.format!=='nu54-firmware-candidate-v1'||claim.model!==model||claim.board!==board.device||
      claim.generation!==board.generation.toString()||claim.measurement!==running.measurement||claim.counter!==running.counter||
      claim.bootKey!==bootPin||typeof issuedAt!=='number'||typeof expiresAt!=='number'||
      !Number.isSafeInteger(issuedAt)||!Number.isSafeInteger(expiresAt)||issuedAt>now||expiresAt<=now||
      expiresAt<=issuedAt||expiresAt-issuedAt>300)
    throw new Error('FIRMWARE_CANDIDATE_SCOPE');
  const release=record(JSON.parse(signedPayload(claim.descriptor,claim.descriptorSignature,trust.publisherKey)),
    'boardRevision,channel,counter,imageSha256,measurement,version');
  const {version,counter}=release;
  if(release.boardRevision!==trust.boardRevision||release.channel!==trust.channel||
      typeof release.imageSha256!=='string'||!digest.test(release.imageSha256)||typeof release.measurement!=='string'||
      !digest.test(release.measurement)||release.measurement===running.measurement||
      typeof counter!=='number'||!Number.isInteger(counter)||counter<=running.counter||counter>65535||
      typeof version!=='string'||!/^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,4})(\+[1-9][0-9]{0,9})?$/.test(version))
    throw new Error('FIRMWARE_CANDIDATE_RELEASE');
  const [major,minor,patch,build=0]=version.split(/[.+]/).map(Number);
  if(major>255||minor>255||patch>65535||build>4294967295)throw new Error('FIRMWARE_CANDIDATE_RELEASE');
  return {version,counter,channel:trust.channel,issuedAt,expiresAt};
}
