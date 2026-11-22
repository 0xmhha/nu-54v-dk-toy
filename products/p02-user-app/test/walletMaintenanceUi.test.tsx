import Renderer from 'react-test-renderer';
import {AppState,TextInput} from 'react-native';
import {WalletHome} from '../src/wallet/WalletHome.tsx';
import {WalletMaintenance} from '../src/wallet/WalletMaintenance.tsx';
import type {RunningFirmware} from '../src/wallet/firmwareStatusWire.ts';
import type {OwnerStatus} from '../src/wallet/ownerWire.ts';
import type {ComponentProps} from 'react';

let mockStorage='';
const mockProbe=jest.fn();
const mockRenterProfile=jest.fn();
const mockRenterExchange=jest.fn();
jest.mock('../src/specs/NativeCommerceHttp.ts',()=>({__esModule:true,default:{
  loadProfile:(slot:string)=>mockRenterProfile(slot),
  exchangeRenterReturn:(...args:string[])=>mockRenterExchange(...args),
}}));
jest.mock('../src/specs/NativeOwnerWallet.ts',()=>({__esModule:true,default:{
  loadProfile:async()=>mockStorage,setHomeProtection:async()=>{},
  compareAndSaveProfile:async(expected:string,value:string)=>{
    if(mockStorage!==expected)return false;
    mockStorage=value;return true;
  },
}}));
jest.mock('../src/specs/NativeRenterBle.ts',()=>({__esModule:true,default:{loadDevices:async()=>'[]'}}));
jest.mock('../src/wallet/ownerSession.ts',()=>({OwnerSession:class {
  async maintenanceStatus(){return mockProbe();}
}}));

const status:OwnerStatus={device:'a'.repeat(64),owner:'b'.repeat(64),generation:4n,root:2,profile:1,backedUp:true,state:'active'};
const defaults:ComponentProps<typeof WalletMaintenance>={
  language:'ko',scope:{mode:'personal',board:status.device,generation:4},active:true,signing:false,unsettled:false,networkBusy:false,
  devices:[{address:'fixture',name:'NU54'}],scan:()=>{},read:async()=>status,campaign:async()=>'fixture-campaign-proof',
  back:()=>{},backup:()=>{},orders:()=>{},
};
const action=(tree:Renderer.ReactTestRenderer,label:string)=>tree.root.findAll(node=>
  node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
const tap=async(tree:Renderer.ReactTestRenderer,label:string)=>Renderer.act(async()=>{
  const button=action(tree,label);
  expect(button.props.disabled).not.toBe(true);
  button.props.onPress();
});

test('W12 rejects campaign reads while signing and refuses personal reset while unsettled',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  const read=jest.fn(async()=>status);
  const campaign=jest.fn(async()=>'fixture-campaign-proof');
  const orders=jest.fn();
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} signing read={read} campaign={campaign} orders={orders}/>);});
    expect(JSON.stringify(tree.toJSON())).toContain('승인 진행 또는 결과 미확인 · 업데이트 적용 보류');
    expect(action(tree,'인증된 기기 상태 조회 · NU54').props.disabled).toBe(true);
    expect(action(tree,'이 기기 적용 증거 읽기').props.disabled).toBe(true);
    expect(read).not.toHaveBeenCalled();
    expect(campaign).not.toHaveBeenCalled();
    await Renderer.act(async()=>{tree.update(<WalletMaintenance {...defaults} unsettled read={read} campaign={campaign} orders={orders}/>);});
    await tap(tree,'개인 초기화 조건 확인');
    expect(JSON.stringify(tree.toJSON())).toContain('미결 지급 또는 서명 요청을 원래 주문에서 먼저 대사하세요');
    await tap(tree,'미결 지급 조회');
    expect(orders).toHaveBeenCalledTimes(1);
    expect(campaign).not.toHaveBeenCalled();
  }finally { await Renderer.act(async()=>tree?.unmount()); }
});

test('W12 drops a previous owner campaign reply after switching to rental and opens only the rental path',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  let finish:(proof:string)=>void=()=>{};
  const campaign=jest.fn(async()=>new Promise<string>(resolve=>{finish=resolve;}));
  const rental=jest.fn();
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} campaign={campaign} rental={rental}/>);});
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    const command='10'+'12'.repeat(48);
    const input=tree.root.findAll(node=>node.props.accessibilityLabel==='펌웨어 적용 증거 요청'&&typeof node.props.onChangeText==='function')[0];
    await Renderer.act(async()=>{input.props.onChangeText(command);});
    await tap(tree,'이 기기 적용 증거 읽기');
    expect(campaign).toHaveBeenCalledWith('fixture',command,status);
    await Renderer.act(async()=>{tree.update(<WalletMaintenance {...defaults} scope={{mode:'rental'}} rental={rental} campaign={campaign}/>);});
    await Renderer.act(async()=>finish('prior-owner-proof'));
    expect(JSON.stringify(tree.toJSON())).not.toContain('prior-owner-proof');
    expect(action(tree,'개인 초기화 조건 확인')).toBeUndefined();
    await tap(tree,'기존 대여 보드 연결');
    expect(rental).toHaveBeenCalledTimes(1);
  }finally { await Renderer.act(async()=>tree?.unmount()); }
});


test('W12 enters from the normal enrolled wallet settings and preserves the signing journal',async()=>{
  const appState=Object.getOwnPropertyDescriptor(AppState,'currentState')!;
  Object.defineProperty(AppState,'currentState',{configurable:true,value:'active'});
  const order=jest.fn();
  const rental=jest.fn();
  const before=JSON.stringify({version:2,mode:'personal',exercised:true,
    verified:{boardFingerprint:status.device,address:'0x'+'12'.repeat(20),generation:4,network:'8283'},
    enrollment:{version:2,device:status.device,boardSPKI:'cd'.repeat(32),ca:'MA==',certificateChain:['MA==']},
    signing:{kind:'chain',digest:'34'.repeat(32),board:status.device,generation:4,expires:Math.floor(Date.now()/1000)+60},
  });
  mockStorage=before;
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome rental={rental} orders={order}
      permission={async()=>true} honorExistingRental={false}/>);});
    await tap(tree,'설정');
    await tap(tree,'기기·업데이트');
    expect(JSON.stringify(tree.toJSON())).toContain('업데이트 적용 보류');
    await tap(tree,'개인 초기화 조건 확인');
    expect(JSON.stringify(tree.toJSON())).toContain('미결 지급 또는 서명 요청');
    await tap(tree,'미결 지급 조회');
    expect(order).toHaveBeenCalledTimes(1);
    expect(mockProbe).not.toHaveBeenCalled();
    expect(rental).not.toHaveBeenCalled();
    expect(mockStorage).toBe(before);
    await tap(tree,'설정으로 돌아가기');
    expect(action(tree,'기기·업데이트')).toBeDefined();
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    Object.defineProperty(AppState,'currentState',appState);
  }
});

test('W12 labels the authenticated running image separately from unknown candidate and power',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  const runningFirmware=jest.fn().mockResolvedValueOnce({
    version:'2.3.258+65538',counter:512,measurement:'dc'.repeat(32),confidence:'confirmed-running',
  }).mockRejectedValueOnce(new Error('OWNER_DISCONNECTED'));
  const campaign=jest.fn();
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} runningFirmware={runningFirmware} campaign={campaign}/>);});
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    expect(runningFirmware).not.toHaveBeenCalled();
    expect(JSON.stringify(tree.toJSON())).not.toContain('현재 실행 버전');
    await tap(tree,'펌웨어 업데이트 준비');
    expect(JSON.stringify(tree.toJSON())).toContain('확인 불가');
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    expect(runningFirmware).toHaveBeenCalledWith('fixture',status);
    expect(JSON.stringify(tree.toJSON())).toContain('2.3.258+65538');
    expect(JSON.stringify(tree.toJSON())).toContain('서명 후보 · 확인 불가');
    expect(JSON.stringify(tree.toJSON())).toContain('기기 전원 · 확인 불가');
    expect(JSON.stringify(tree.toJSON())).not.toContain('dc'.repeat(32));
    expect(campaign).not.toHaveBeenCalled();
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    expect(JSON.stringify(tree.toJSON())).not.toContain('2.3.258+65538');
    expect(JSON.stringify(tree.toJSON())).toContain('확인 불가');
    expect(campaign).not.toHaveBeenCalled();
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 discards a measured image when the enrolled generation changes during its read',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  let finish!:()=>void;
  const runningFirmware=jest.fn(async()=>new Promise<RunningFirmware>(resolve=>{
    finish=()=>resolve({version:'2.3.4',counter:8,measurement:'cd'.repeat(32),confidence:'confirmed-running'});
  }));
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} runningFirmware={runningFirmware}/>);});
    await tap(tree,'펌웨어 업데이트 준비');
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    await Renderer.act(async()=>tree.update(<WalletMaintenance {...defaults} scope={{...defaults.scope,generation:5}} runningFirmware={runningFirmware}/>));
    await Renderer.act(async()=>finish());
    expect(JSON.stringify(tree.toJSON())).not.toContain('2.3.4');
    expect(JSON.stringify(tree.toJSON())).toContain('확인 불가');
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 checks the PMIC only when requested and keeps battery and update permission separate',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  const inputPower=jest.fn().mockResolvedValueOnce('vin-good').mockResolvedValueOnce('vin-not-good')
    .mockRejectedValueOnce(new Error('OWNER_DISCONNECTED'));
  const statusRead=jest.fn(async()=>status);
  const runningFirmware=jest.fn(async()=>({version:'2.3.4',counter:8,measurement:'cd'.repeat(32),confidence:'confirmed-running' as const}));
  const campaign=jest.fn();
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} read={statusRead}
      runningFirmware={runningFirmware} inputPower={inputPower} campaign={campaign}/>);});
    await tap(tree,'펌웨어 업데이트 준비');
    expect(action(tree,'PMIC 입력 확인').props.disabled).toBe(true);
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    expect(runningFirmware).toHaveBeenCalledTimes(1);
    expect(inputPower).not.toHaveBeenCalled();
    await tap(tree,'PMIC 입력 확인');
    expect(inputPower).toHaveBeenCalledWith('fixture',status);
    let shown=JSON.stringify(tree.toJSON());
    expect(shown).toContain('VIN 정상 범위');
    expect(shown).toContain('기기 전원 · 확인 불가');
    expect(shown).toContain('배터리 잔량 · 확인 불가');
    expect(shown).toContain('업데이트 적용 권한 · 보류');
    expect(campaign).not.toHaveBeenCalled();
    await tap(tree,'PMIC 입력 확인');
    expect(JSON.stringify(tree.toJSON())).toContain('VIN 조건 미충족');
    await tap(tree,'PMIC 입력 확인');
    shown=JSON.stringify(tree.toJSON());
    expect(shown).not.toContain('VIN 정상 범위');
    expect(shown).toContain('확인 불가');
    expect(shown).toContain('2.3.4');
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 drops a late PMIC read when the wallet changes owner scope',async()=>{
  let tree!:Renderer.ReactTestRenderer;
  let publish!:()=>void;
  const inputPower=jest.fn(async()=>new Promise<'vin-good'>(resolve=>{publish=()=>resolve('vin-good');}));
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} inputPower={inputPower}/>);});
    await tap(tree,'펌웨어 업데이트 준비');
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    await tap(tree,'PMIC 입력 확인');
    await Renderer.act(async()=>tree.update(<WalletMaintenance {...defaults} scope={{...defaults.scope,generation:5}}
      inputPower={inputPower}/>));
    await Renderer.act(async()=>publish());
    expect(JSON.stringify(tree.toJSON())).not.toContain('VIN 정상 범위');
    expect(action(tree,'PMIC 입력 확인').props.disabled).toBe(true);
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 opens return from the rental profile without private board actions',async()=>{
  mockStorage=JSON.stringify({version:2,mode:'legacy',exercised:true});
  const rental=jest.fn();
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome rental={rental} orders={()=>{}}
      permission={async()=>true} honorExistingRental={false}/>);});
    await tap(tree,'대여 보드 반납 상태');
    expect(JSON.stringify(tree.toJSON())).toContain('인증된 운영자가 원래 대여 지급을 대사한 뒤');
    expect(JSON.stringify(tree.toJSON())).toContain('대여 슬롯의 종료 확인을 받지 못했습니다.');
    expect(JSON.stringify(tree.toJSON())).not.toContain('대여 권한이 종료되었습니다');
    expect(action(tree,'미결 지급 조회')).toBeUndefined();
    expect(action(tree,'개인 초기화 조건 확인')).toBeUndefined();
    expect(action(tree,'이 기기 적용 증거 읽기')).toBeUndefined();
    await tap(tree,'English');
    expect(JSON.stringify(tree.toJSON())).toContain('Closure of the rental slot has not been confirmed.');
    expect(JSON.stringify(tree.toJSON())).not.toContain('Rental authority has ended');
    expect(action(tree,'View pending payments')).toBeUndefined();
    expect(rental).not.toHaveBeenCalled();
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 separates board find from phone audio and can cancel the acknowledged signal',async()=>{
  const tools=jest.fn(async()=>true);
  const find=jest.fn(async()=>true);
  const speak=jest.fn(async()=>{});
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} tools={tools} find={find} speak={speak}/>);});
    await tap(tree,'기기 부가기능');
    expect(action(tree,'찾기 신호 요청').props.disabled).toBe(true);
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    expect(tools).toHaveBeenCalledWith('fixture',status);
    expect(JSON.stringify(tree.toJSON())).toContain('인증된 기기 지문 끝자리');
    expect(JSON.stringify(tree.toJSON())).toContain('AAAA');
    await tap(tree,'찾기 신호 요청');
    expect(find).toHaveBeenLastCalledWith('fixture',status,true);
    await tap(tree,'찾기 신호 취소');
    expect(find).toHaveBeenLastCalledWith('fixture',status,false);
    await tap(tree,'휴대전화로 상태 듣기');
    expect(speak).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(tree.toJSON())).toContain('보드 오디오 전송 미지원');
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 expires the displayed signal and drops a late result when owner scope changes',async()=>{
  jest.useFakeTimers();
  let tree!:Renderer.ReactTestRenderer;
  let complete:(value:boolean)=>void=()=>{};
  const pending=()=>new Promise<boolean>(resolve=>{complete=resolve;});
  const find=jest.fn(async()=>true);
  const tools=async()=>true;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletMaintenance {...defaults} tools={tools} find={find}/>);});
    await tap(tree,'기기 부가기능');
    await tap(tree,'인증된 기기 상태 조회 · NU54');
    await tap(tree,'찾기 신호 요청');
    expect(action(tree,'찾기 신호 취소').props.disabled).toBe(false);
    await Renderer.act(async()=>{jest.advanceTimersByTime(4500);});
    expect(action(tree,'찾기 신호 취소').props.disabled).toBe(true);
    find.mockImplementationOnce(pending);
    await tap(tree,'찾기 신호 요청');
    await Renderer.act(async()=>{tree.update(<WalletMaintenance {...defaults} scope={{mode:'rental'}} tools={tools} find={find}/>);});
    await Renderer.act(async()=>complete(true));
    expect(JSON.stringify(tree.toJSON())).not.toContain('LED 신호 요청을 받았습니다');
    expect(action(tree,'찾기 신호 요청')).toBeUndefined();
  } finally {
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});

test('W09 opens the sole rental original in common wallet navigation and clears it after profile replacement',async()=>{
  jest.useFakeTimers();
  const originalAppState=Object.getOwnPropertyDescriptor(AppState,'currentState')!;
  Object.defineProperty(AppState,'currentState',{configurable:true,value:'active'});
  const storedWallet=JSON.stringify({version:2,mode:'legacy',exercised:true});
  mockStorage=storedWallet;
  mockRenterProfile.mockReset().mockResolvedValue('current-renter');
  mockRenterExchange.mockReset();
  const ref='8ddb7d93-34c0-4312-9f20-941ff1215863';
  const origin='c51da18b-566f-437d-9e6e-1471633a1f58';
  mockRenterExchange.mockResolvedValue(JSON.stringify({
    assignment:ref,account:'0x'+'ab'.repeat(20),generation:'3',assignmentUntil:new Date(Date.now()+9000).toISOString(),
    slot:'unconfirmed',settlement:'verified',pendingCount:2,pendingPayments:1,pendingLimits:1,
    basisAt:new Date(Date.now()-100).toISOString(),basisUntil:new Date(Date.now()+5000).toISOString(),
    pendingOrders:[{origin,amount:'2000000',asset:'0x'+'cd'.repeat(20)}],tokenDecimals:6,tokenSymbol:'tUSDC',
  }));
  const genericOrders=jest.fn();
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome rental={()=>{}} orders={genericOrders}
      permission={async()=>true} honorExistingRental={false}/>);});
    await tap(tree,'대여 보드 반납 상태');
    await Renderer.act(async()=>tree.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='대여 인도 참조')!.props.onChangeText(ref));
    await tap(tree,'같은 대여 상태 조회');
    await tap(tree,'미결 원거래 '+origin);
    expect(JSON.stringify(tree.toJSON())).toContain('원래 미결 지급');
    expect(JSON.stringify(tree.toJSON())).toContain('2.000000');
    expect(action(tree,'같은 대여 상태 조회')).toBeUndefined();
    expect(genericOrders).not.toHaveBeenCalled();
    await tap(tree,'대여 반납 상태로 돌아가기');
    expect(action(tree,'같은 대여 상태 조회')).toBeDefined();
    const beforeExit=mockRenterExchange.mock.calls.length;
    expect(action(tree,'프로필 선택').props.accessibilityRole).toBe('button');
    await tap(tree,'프로필 선택');
    expect(JSON.stringify(tree.toJSON())).toContain('어떤 지갑을 사용할까요?');
    expect(JSON.stringify(tree.toJSON())).not.toContain(ref);
    expect(action(tree,'같은 대여 상태 조회')).toBeUndefined();
    expect(mockRenterExchange).toHaveBeenCalledTimes(beforeExit);
    expect(mockStorage).toBe(storedWallet);
    await tap(tree,'대여 보드 반납 상태');
    expect(action(tree,'같은 대여 상태 조회').props.disabled).toBe(true);
    await Renderer.act(async()=>tree.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='대여 인도 참조')!.props.onChangeText(ref));
    await tap(tree,'같은 대여 상태 조회');
    expect(mockRenterExchange).toHaveBeenCalledTimes(beforeExit+1);
    expect(mockRenterExchange.mock.lastCall).toEqual(mockRenterExchange.mock.calls[0]);
    expect(action(tree,'미결 원거래 '+origin)).toBeDefined();
    await tap(tree,'미결 원거래 '+origin);
    mockRenterProfile.mockResolvedValue('replaced-renter');
    await Renderer.act(async()=>{jest.advanceTimersByTime(1001);});
    expect(JSON.stringify(tree.toJSON())).not.toContain(origin);
    expect(action(tree,'같은 대여 상태 조회').props.disabled).toBe(true);
    expect(mockRenterExchange.mock.calls.every(call=>call[1]===ref&&call[2]==='')).toBe(true);
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    Object.defineProperty(AppState,'currentState',originalAppState);
    jest.useRealTimers();
  }
});
