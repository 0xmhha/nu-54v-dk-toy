import Renderer from 'react-test-renderer';
import {WalletMaintenance} from '../src/wallet/WalletMaintenance.tsx';
import {ed25519} from '@noble/curves/ed25519';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,hexToBytes,utf8Encode} from '@nu54/protocol';
import {candidateTrustForInstallation,verifyFirmwareCandidate,type FirmwareCandidateTrust} from '../src/wallet/firmwareCandidate.ts';
import type {OwnerStatus} from '../src/wallet/ownerWire.ts';
import type {RunningFirmware} from '../src/wallet/firmwareStatusWire.ts';

const station=Uint8Array.from({length:32},()=>7);
const publisher=Uint8Array.from({length:32},()=>8);
const boot=bytesToHex(ed25519.getPublicKey(Uint8Array.from({length:32},()=>9)),false);
const board='12'.repeat(32);
const now=1800000000;
const trust:FirmwareCandidateTrust={board,boardRevision:'nu54_rev1',channel:'custom',bootKey:boot,
  stationKey:bytesToHex(ed25519.getPublicKey(station),false),
  publisherKey:bytesToHex(ed25519.getPublicKey(publisher),false)};
const status={state:'active',device:board,generation:3n} as OwnerStatus;
const running:RunningFirmware={version:'1.0.0',counter:512,measurement:'34'.repeat(32),confidence:'confirmed-running'};
const release={boardRevision:'nu54_rev1',channel:'custom',counter:513,version:'2.0.0',imageSha256:'56'.repeat(32),measurement:'78'.repeat(32)};
const claim={format:'nu54-firmware-candidate-v1',model:'nu54v_dk/nrf54l15/cpuapp',board,generation:'3',
  bootKey:bytesToHex(sha256(hexToBytes('302a300506032b6570032100'+boot)),false),
  counter:running.counter,measurement:running.measurement,issuedAt:now,expiresAt:now+30};
const sign=(value:string,key:Uint8Array)=>bytesToHex(ed25519.sign(utf8Encode(value),key),false);

function candidate(changes:Record<string,unknown>={}, releaseChanges:Record<string,unknown>={}) {
  const descriptor=JSON.stringify({...release,...releaseChanges})+'\n';
  const payload=JSON.stringify({...claim,descriptor,descriptorSignature:sign(descriptor,publisher),...changes});
  return JSON.stringify({payload,signature:sign(payload,station)});
}

test('requires both independent signatures and binds the selected release to the live board and next counter',()=>{
  expect(verifyFirmwareCandidate(candidate(),trust,status,running,now)).toEqual({
    version:'2.0.0',counter:513,channel:'custom',issuedAt:now,expiresAt:now+30,
  });
  for(const raw of [
    candidate({descriptorSignature:'ab'.repeat(64)}),candidate({generation:'4'}),candidate({board:'13'.repeat(32)}),
    candidate({counter:511}),candidate({bootKey:'11'.repeat(32)}),candidate({measurement:'41'.repeat(32)}),
    candidate({expiresAt:now}),candidate({issuedAt:now+1}),
    candidate({},{boardRevision:'nu54_rev2'}),candidate({},{channel:'official'}),
    candidate({},{counter:512}),candidate({},{imageSha256:'bad'}),
  ]) expect(()=>verifyFirmwareCandidate(raw,trust,status,running,now)).toThrow();
  expect(()=>verifyFirmwareCandidate(candidate(),{...trust,stationKey:trust.publisherKey},status,running,now)).toThrow();
  expect(()=>verifyFirmwareCandidate(candidate(),trust,{...status,generation:4n},running,now)).toThrow();
  expect(()=>verifyFirmwareCandidate(candidate(),trust,status,running,now+30)).toThrow();
});


test('the normal maintenance panel clears a cancelled, mismatched or superseded candidate',async()=>{
  const choose=jest.fn<Promise<string>,[]>();
  const read=jest.fn(async()=>status);
  const runningFirmware=jest.fn(async()=>running);
  const campaign=jest.fn(async()=>'');
  const props={language:'ko' as const,scope:{mode:'personal' as const,board,generation:3},active:true,
    signing:false,unsettled:false,networkBusy:false,devices:[{address:'device',name:'NU54'}],scan:()=>{},
    read,runningFirmware,candidateTrust:trust,pickCandidate:choose,campaign,back:()=>{},backup:()=>{},orders:()=>{}};
  let tree!:Renderer.ReactTestRenderer;
  const date=jest.spyOn(Date,'now').mockReturnValue(now*1000);
  const rendered=()=>JSON.stringify(tree.toJSON());
  const control=(label:string)=>tree.root.findAll(item=>item.props.accessibilityLabel===label&&typeof item.props.onPress==='function')[0];
  const tap=async(label:string)=>Renderer.act(async()=>{
    expect(control(label).props.disabled).not.toBe(true);
    control(label).props.onPress();
  });
  const picker='서명된 후보 확인서 선택';
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...props}/>);});
    await tap('펌웨어 업데이트 준비');
    await tap('인증된 기기 상태 조회 · NU54');
    expect(rendered()).toContain('서명 후보 · 확인 불가');
    expect(choose).not.toHaveBeenCalled();
    expect(runningFirmware).toHaveBeenCalledTimes(1);
    choose.mockResolvedValueOnce(candidate());
    await tap(picker);
    expect(rendered()).toContain('2.0.0 · custom · 보안 카운터 513');
    expect(runningFirmware).toHaveBeenCalledTimes(2);
    await tap('후보 선택 취소');
    expect(rendered()).toContain('서명 후보 · 확인 불가');
    choose.mockRejectedValueOnce(new Error('FILE_CANCELLED'));
    await tap(picker);
    expect(rendered()).toContain('서명 후보 · 확인 불가');
    expect(runningFirmware).toHaveBeenCalledTimes(2);
    choose.mockResolvedValueOnce(candidate({},{boardRevision:'other'}));
    await tap(picker);
    expect(rendered()).toContain('서명 후보가 현재 기기 및 배포 정책과 일치하지 않습니다.');
    let finish:(raw:string)=>void=()=>{};
    choose.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
    await tap(picker);
    await Renderer.act(async()=>{tree.update(<WalletMaintenance {...props} unsettled/>);});
    await Renderer.act(async()=>finish(candidate()));
    expect(rendered()).toContain('서명 후보 · 확인 불가');
    expect(control(picker)).toBeUndefined();
    expect(campaign).not.toHaveBeenCalled();
    date.mockReturnValue((now+31)*1000);
    expect(rendered()).not.toContain('2.0.0');
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    date.mockRestore();
  }
});

test('only the original board’s historical installation can contradict independently deployed trust',()=>{
  const original={device:board,channel:'custom' as const,bootKey:claim.bootKey};
  expect(candidateTrustForInstallation(trust,original)).toBe(trust);
  expect(candidateTrustForInstallation(trust,{...original,channel:'official'})).toBeUndefined();
  expect(candidateTrustForInstallation(trust,{...original,bootKey:trust.bootKey})).toBeUndefined();
  expect(candidateTrustForInstallation(trust,{...original,device:'c8'.repeat(32),bootKey:trust.bootKey})).toBe(trust);
  expect(candidateTrustForInstallation(undefined,original)).toBeUndefined();
  const official={...trust,channel:'official'} as unknown as FirmwareCandidateTrust;
  expect(candidateTrustForInstallation(official,undefined)).toBeUndefined();
  expect(()=>verifyFirmwareCandidate(candidate(),official,status,running,now)).toThrow('FIRMWARE_CANDIDATE_SCOPE');
});
