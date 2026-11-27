import {bigToBytes, bytesToBig, bytesToHex, concat} from '@nu54/protocol';

const powerOperation=0x78;

export type FirmwareInputPower='unknown'|'vin-not-good'|'vin-good';

export function firmwarePowerRequest(generation:bigint) {
  if (generation<=0n || generation>0xffffffffffffffffn) throw new Error('FIRMWARE_POWER_SCOPE');
  return concat(Uint8Array.of(powerOperation),bigToBytes(generation,8));
}

export function readFirmwareInputPower(reply:Uint8Array, board:string, generation:bigint):FirmwareInputPower {
  firmwarePowerRequest(generation);
  if (reply.length!==43 || reply[0]!==powerOperation || reply[1]!==1 ||
      bytesToBig(reply.subarray(3,11))!==generation || bytesToHex(reply.subarray(11,43),false)!==board ||
      !reply.subarray(11).some(Boolean)) throw new Error('FIRMWARE_POWER_SCOPE');
  if (reply[2]===1) return 'vin-not-good';
  if (reply[2]===2) return 'vin-good';
  if (reply[2]===0) return 'unknown';
  throw new Error('FIRMWARE_POWER_STATUS');
}
