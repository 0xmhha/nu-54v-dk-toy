import {hexToBytes} from '@nu54/protocol';
import {readRunningFirmware,runningFirmwareRequest} from '../src/wallet/firmwareStatusWire';

const board='ab'.repeat(32), measurement='dc'.repeat(32);
const generation=257n;
const reply=hexToBytes('77010203010200010002000002000000000000000101'+board+measurement);

test('W12 binds the running image version and protected counter to the authenticated owner scope',()=>{
  expect(runningFirmwareRequest(generation)).toEqual(hexToBytes('770000000000000101'));
  expect(readRunningFirmware(reply,board,generation)).toEqual({
    version:'2.3.258+65538',counter:512,measurement,confidence:'confirmed-running',
  });
  expect(()=>readRunningFirmware(reply,'cd'.repeat(32),generation)).toThrow('FIRMWARE_STATUS_SCOPE');
  expect(()=>readRunningFirmware(reply,board,258n)).toThrow('FIRMWARE_STATUS_SCOPE');
  expect(()=>runningFirmwareRequest(0n)).toThrow('FIRMWARE_STATUS_SCOPE');
});

test('W12 refuses missing measurement, missing counter and post-boot campaign bytes',()=>{
  expect(readRunningFirmware(Uint8Array.of(0x77),board,generation)).toBeUndefined();
  expect(()=>readRunningFirmware(reply.slice(0,-1),board,generation)).toThrow('FIRMWARE_STATUS_SCOPE');
  const noMeasurement=reply.slice();noMeasurement.fill(0,54);
  expect(()=>readRunningFirmware(noMeasurement,board,generation)).toThrow('FIRMWARE_STATUS_SCOPE');
  const noCounter=reply.slice();noCounter.fill(0,10,14);
  expect(()=>readRunningFirmware(noCounter,board,generation)).toThrow('FIRMWARE_STATUS_COUNTER');
  const campaign=new Uint8Array(181);campaign[0]=16;
  expect(()=>readRunningFirmware(campaign,board,generation)).toThrow('FIRMWARE_STATUS_SCOPE');
});
