import {bigToBytes, bytesToBig, bytesToHex, concat} from '@nu54/protocol';

export const runningFirmwareOperation=0x77;

export type RunningFirmware = {
  version:string;
  counter:number;
  measurement:string;
  confidence:'confirmed-running';
};

export function runningFirmwareRequest(generation:bigint) {
  if (generation<=0n || generation>0xffffffffffffffffn) throw new Error('FIRMWARE_STATUS_SCOPE');
  return concat(Uint8Array.of(runningFirmwareOperation),bigToBytes(generation,8));
}

export function readRunningFirmware(reply:Uint8Array, board:string, generation:bigint):RunningFirmware|undefined {
  runningFirmwareRequest(generation);
  if (reply.length===1 && reply[0]===runningFirmwareOperation) return undefined;
  if (reply.length!==86 || reply[0]!==runningFirmwareOperation || reply[1]!==1 ||
      bytesToBig(reply.subarray(14,22))!==generation || bytesToHex(reply.subarray(22,54),false)!==board ||
      !reply.subarray(22,54).some(Boolean) || !reply.subarray(54,86).some(Boolean))
    throw new Error('FIRMWARE_STATUS_SCOPE');
  const view = new DataView(reply.buffer,reply.byteOffset,reply.byteLength);
  const counter=view.getUint32(10);
  if (!counter) throw new Error('FIRMWARE_STATUS_COUNTER');
  const build = view.getUint32(6);
  return {version:`${reply[2]}.${reply[3]}.${view.getUint16(4)}${build ? `+${build}` : ''}`,
    counter,measurement:bytesToHex(reply.subarray(54,86),false),confidence:'confirmed-running'};
}
