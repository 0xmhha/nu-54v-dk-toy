import {hexToBytes} from '@nu54/protocol';
import {firmwarePowerRequest,readFirmwareInputPower} from '../src/wallet/firmwarePowerWire';

const board='ab'.repeat(32), generation=257n;

test('W12 VIN observations are current-generation PMIC facts only',()=>{
  expect(firmwarePowerRequest(generation)).toEqual(hexToBytes('780000000000000101'));
  for(const [value,status] of [['00','unknown'],['01','vin-not-good'],['02','vin-good']] as const)
    expect(readFirmwareInputPower(hexToBytes('7801'+value+'0000000000000101'+board),board,generation)).toBe(status);
  expect(()=>readFirmwareInputPower(hexToBytes('7801030000000000000101'+board),board,generation)).toThrow('FIRMWARE_POWER_STATUS');
});

test('W12 rejects malformed, other-board and older-generation VIN evidence',()=>{
  const reply=hexToBytes('7801020000000000000101'+board);
  expect(()=>readFirmwareInputPower(reply,'cd'.repeat(32),generation)).toThrow('FIRMWARE_POWER_SCOPE');
  expect(()=>readFirmwareInputPower(reply,board,258n)).toThrow('FIRMWARE_POWER_SCOPE');
  expect(()=>readFirmwareInputPower(reply.slice(0,-1),board,generation)).toThrow('FIRMWARE_POWER_SCOPE');
  expect(()=>firmwarePowerRequest(0n)).toThrow('FIRMWARE_POWER_SCOPE');
});
