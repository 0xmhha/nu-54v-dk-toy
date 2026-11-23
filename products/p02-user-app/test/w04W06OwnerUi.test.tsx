import React from 'react';
import Renderer from 'react-test-renderer';
import {AccessibilityInfo,BackHandler,Keyboard,Linking,Platform,TextInput} from 'react-native';
import {WalletHome} from '../src/wallet/WalletHome';
import App from '../src/App';
import {bytesToHex,concat,hexToBytes} from '@nu54/protocol';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
jest.mock('react-native-safe-area-context',()=>require('react-native-safe-area-context/jest/mock').default);
let mockRecoveryCloudProfile='';
const mockRecoveryCloudRequest=jest.fn(),mockScanOwner=jest.fn();
jest.mock('../src/specs/NativeCloudWalletProfile.ts',()=>({__esModule:true,default:{
 profile:async()=>mockRecoveryCloudProfile,request:(...args: unknown[])=>mockRecoveryCloudRequest(...args),
}}));
let mockStored='',mockBackupReady=false,mockBackupUnavailable=false,mockVerify='committed',mockReverse=false,mockBondsClear=false;
let mockRegisterGate:Promise<void>=Promise.resolve();
const mockConnect=jest.fn(),mockRegister=jest.fn(),mockVerifyLost=jest.fn(),mockBeginRecovery=jest.fn();
const initialPlatform=Platform.OS;afterAll(()=>Object.defineProperty(Platform,'OS',{value:initialPlatform,configurable:true}));
const mockDevice='34'.repeat(32),mockOwner='ab'.repeat(32),mockAddress='0x'+'12'.repeat(20);
jest.mock('../src/specs/NativeOwnerWallet.ts',()=>({__esModule:true,default:{setHomeProtection:async()=>{},loadProfile:async()=>mockStored,
 compareAndSaveProfile:async(expected:string,next:string)=>{if(expected!==mockStored)return false;mockStored=next;return true;}}}));
jest.mock('../src/specs/NativeRenterBle.ts',()=>({__esModule:true,default:{loadDevices:async()=>'[]',
 onScan:(listener:(value:string)=>void)=>{for(const address of (mockReverse?['right-222222','wrong-111111']:['wrong-111111','right-222222']))listener(JSON.stringify({name:'NU54V',address}));return{remove(){}};},scanOwner:async()=>mockScanOwner()}}));
jest.mock('../src/wallet/ownerSession.ts',()=>({OwnerSession:class {
 handle='';async connect(handle:string){this.handle=handle;mockConnect(handle);if(handle.startsWith('wrong'))throw Error('OWNER_PIN');}close(){}
 async identity(){return{fingerprint:mockOwner};}async status(){return{state:'active',device:mockDevice,owner:mockOwner,generation:1n,root:2,profile:1,backedUp:mockBackupReady};}
 async maintenanceStatus(){return this.status();}
 async beginRecovery(){mockBeginRecovery(this.handle);return{requestId:'11'.repeat(16),challenge:'22',expires:'9999999999'};}
 async register(confirm:()=>void){mockRegister();confirm();await mockRegisterGate;return this.status();}
 async backedUpAddress(){return mockAddress;}
 async beginBackup(){if(mockBackupUnavailable)throw Error('BACKUP_UNAVAILABLE');return{requestId:'56'.repeat(16),expires:'9999999999',checkpoint:{device:mockDevice,owner:mockOwner,generation:'1'}};}async checkBackup(){return mockBackupReady?{verified:true,address:mockAddress,status:await this.status()}:{verified:false};}
 async verifyLostPhone(){mockVerifyLost();if(mockVerify==='not-committed')throw Error('OWNER_REPLACEMENT_NOT_COMMITTED');return{state:'active',device:mockDevice,owner:mockOwner,generation:2n,root:2,profile:1,backedUp:true,legacyBondsClear:mockBondsClear};}
}}));
const profile=()=>({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:mockDevice,address:mockAddress,generation:1,network:'8283'},
 enrollment:{version:2,device:mockDevice,boardSPKI:'24'.repeat(32),ca:'MA==',certificateChain:['MA==']}});
async function openTab(tree:Renderer.ReactTestRenderer,label:string){await Renderer.act(async()=>{tree.root.find(node=>node.props.accessibilityRole==='tab'&&node.props.accessibilityLabel===label).props.onPress();});}
async function tap(tree:Renderer.ReactTestRenderer,label:string,index=0){await Renderer.act(async()=>{const items=tree.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function');expect(items[index]).toBeDefined();items[index].props.onPress();});}

test('W04 shows the board step and buttons while registration waits on the screenless board',async()=>{
 mockStored=JSON.stringify({...profile(),verified:undefined});mockRegister.mockClear();
 let release=()=>{};mockRegisterGate=new Promise<void>(resolve=>{release=resolve;});
 let tree:Renderer.ReactTestRenderer;const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 const panel=()=>tree!.root.findAll(node=>node.props.testID==='board-step-panel');
 const shown=()=>JSON.stringify(panel().map(node=>node.findAll(child=>typeof child.props.children==='string').map(child=>child.props.children)));
 try{await Renderer.act(async()=>{tree=Renderer.create(app);});
  expect(shown()).toContain('보드의 SW4를 1초 이상 길게 누르세요.');expect(shown()).toContain('1 / 4');
  await tap(tree!,'개인 기기 찾기');await tap(tree!,'기기 등록 시작',1);
  expect(mockRegister).toHaveBeenCalledTimes(1);
  expect(shown()).toContain('승인하려면 SW1, 거절하려면 SW2를 짧게 누르세요.');expect(shown()).toContain('2 / 4');
  expect(shown()).toContain('이어서 할 일');expect(shown()).not.toContain('연결 창을 열면');
  await Renderer.act(async()=>{release();await mockRegisterGate;});
  expect(panel()).toHaveLength(0);
 }finally{mockRegisterGate=Promise.resolve();await Renderer.act(async()=>tree?.unmount());}
});

test.each(['ios','android'] as const)('W04/W05 %s distinguishes scan candidates and waits for backup proof',async platform=>{
 Object.defineProperty(Platform,'OS',{value:platform,configurable:true});mockStored=JSON.stringify({...profile(),verified:undefined});mockBackupReady=false;mockConnect.mockClear();mockRegister.mockClear();
 let tree:Renderer.ReactTestRenderer;const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 try{await Renderer.act(async()=>{tree=Renderer.create(app);});await tap(tree!,'개인 기기 찾기');
  expect(JSON.stringify(tree!.toJSON())).toContain('검색 이름과 끝자리는 지문이 아닙니다');
  await tap(tree!,'기기 등록 시작',0);expect(mockConnect).toHaveBeenCalledWith('wrong-111111');expect(mockRegister).not.toHaveBeenCalled();
  await tap(tree!,'기기 등록 시작',1);expect(mockRegister).toHaveBeenCalledTimes(1);
  await tap(tree!,'독립 백업 시작');await tap(tree!,'개인 기기 찾기');await tap(tree!,'원본 기기에 백업 요청',1);
  await tap(tree!,'원본 기기의 백업 상태 확인');expect(JSON.stringify(tree!.toJSON())).toContain('독립 복구 확인을 기다리고 있습니다');
  expect(JSON.parse(mockStored).verified).toBeUndefined();
  expect(JSON.stringify(tree!.toJSON())).toContain('입금 안내가 잠겨 있습니다');
  expect(JSON.stringify(tree!.toJSON())).toContain('보관한 기준 주소와 파생 경로');
  mockBackupReady=true;await tap(tree!,'원본 기기의 백업 상태 확인');expect(JSON.stringify(tree!.toJSON())).toContain('원본 기기가 독립 복원 시험 완료를 확인했습니다');
  expect(JSON.parse(mockStored).verified).toMatchObject({address:mockAddress,generation:1,boardFingerprint:mockDevice});
 }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W05 unsupported recovery stays locked on restart and retry uses the enrolled source',async()=>{
 Object.defineProperty(Platform,'OS',{value:'ios',configurable:true});
 mockStored=JSON.stringify({...profile(),verified:undefined});
 const original=mockStored;
 mockBackupReady=false;mockBackupUnavailable=true;
 let tree:Renderer.ReactTestRenderer;
 const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 try{
  await Renderer.act(async()=>{tree=Renderer.create(app);});
  await tap(tree!,'독립 백업 시작');await tap(tree!,'개인 기기 찾기');await tap(tree!,'원본 기기에 백업 요청',1);
  const output=JSON.stringify(tree!.toJSON());
  expect(output).toContain('입금 안내가 잠겨 있습니다');
  expect(output).toContain('기존 활성 슬롯 — 보존');
  expect(tree!.root.findAllByType(TextInput)).toHaveLength(0);
  expect(mockStored).toBe(original);
  await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});
  for(const label of ['다른 체인 받기','개인 전송']){
   const action=tree!.root.findAll(node=>node.props.accessibilityLabel===label && typeof node.props.onPress==='function');
   expect(action.some(node=>node.props.disabled===true||node.props.accessibilityState?.disabled===true)).toBe(true);
  }
  await tap(tree!,'독립 백업 시작');await tap(tree!,'개인 기기 찾기');await tap(tree!,'원본 기기에 백업 요청',1);
  await tap(tree!,'다시 확인');mockBackupUnavailable=false;
  await tap(tree!,'원본 기기에 백업 요청',1);
  expect(JSON.parse(mockStored).backupCheckpoint).toEqual({device:mockDevice,owner:mockOwner,generation:'1'});
  expect(JSON.parse(mockStored).verified).toBeUndefined();
  expect(JSON.stringify(tree!.toJSON())).toContain('입금 안내가 잠겨 있습니다');
  expect(JSON.stringify(tree!.toJSON())).toContain('보관한 기준 주소와 파생 경로');
  mockBackupReady=true;await tap(tree!,'원본 기기의 백업 상태 확인');
  expect(JSON.parse(mockStored).verified.address).toBe(mockAddress);
 }finally{mockBackupUnavailable=false;await Renderer.act(async()=>tree?.unmount());}
});

test.each([['ios',false],['android',false],['ios',true],['android',true]] as const)('W06 %s resumes generation change and preserves the original address (bond store empty: %s)',async(platform,bondsClear)=>{
 Object.defineProperty(Platform,'OS',{value:platform,configurable:true});mockBackupReady=true;mockVerify='committed';mockBondsClear=bondsClear;mockVerifyLost.mockClear();
 mockStored=JSON.stringify({...profile(),phoneReplacement:{device:mockDevice,replacement:mockOwner,generation:'1',address:mockAddress}});
 let tree:Renderer.ReactTestRenderer;const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 try{await Renderer.act(async()=>{tree=Renderer.create(app);});await tap(tree!,'개인 기기 찾기');await tap(tree!,'교체 후 보드 상태 확인',1);
  expect(mockVerifyLost).toHaveBeenCalledTimes(1);const saved=JSON.parse(mockStored);expect(saved.verified).toMatchObject({address:mockAddress,generation:2,boardFingerprint:mockDevice});expect(saved.phoneReplacement).toBeUndefined();
  const screen=JSON.stringify(tree!.toJSON());
  // The board reports an empty legacy bond store only after it removed every bond.
  expect(screen).toContain(bondsClear ? '보드에 남은 이전 휴대전화의 BLE 연결 기록이 없습니다.' : '이전 휴대전화의 BLE 연결 기록은 아직 확인하지 못했습니다.');
  expect(screen).not.toContain(bondsClear ? '아직 확인하지 못했습니다' : '연결 기록이 없습니다');
 }finally{await Renderer.act(async()=>{mockBondsClear=false;tree?.unmount();});}
});

test('W06 pre-commit interruption keeps old owner and offers a fresh explicit recovery',async()=>{
 Object.defineProperty(Platform,'OS',{value:'ios',configurable:true});mockVerify='not-committed';
 mockStored=JSON.stringify({...profile(),phoneReplacement:{device:mockDevice,replacement:mockOwner,generation:'1',address:mockAddress}});
 let tree:Renderer.ReactTestRenderer;const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 try{await Renderer.act(async()=>{tree=Renderer.create(app);});await tap(tree!,'개인 기기 찾기');await tap(tree!,'교체 후 보드 상태 확인',1);
  const saved=JSON.parse(mockStored);expect(saved.verified).toMatchObject({address:mockAddress,generation:1});expect(saved.phoneReplacement).toBeUndefined();
  expect(JSON.stringify(tree!.toJSON())).toContain('교체를 다시 요청할 수 있습니다');
  await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});expect(JSON.parse(mockStored).verified.generation).toBe(1);
 }finally{await Renderer.act(async()=>tree?.unmount());}
});


test('secure subflow hides global navigation and Android back restores owner navigation',async()=>{
 Object.defineProperty(Platform,'OS',{value:'android',configurable:true});mockStored=JSON.stringify(profile());
 let hardware:(()=>boolean)|undefined;const back=jest.spyOn(BackHandler,'addEventListener').mockImplementation((_name,callback)=>{hardware=()=>callback({type:'hardwareBackPress',timeStamp:0})===true;return{remove(){hardware=undefined;}};});
 const onPracticeChange=jest.fn();let tree:Renderer.ReactTestRenderer;
 try{await Renderer.act(async()=>{tree=Renderer.create(<WalletHome onPracticeChange={onPracticeChange} permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
  expect(onPracticeChange).toHaveBeenLastCalledWith(false);await tap(tree!,'독립 백업 시작');expect(onPracticeChange).toHaveBeenLastCalledWith(true);
  await Renderer.act(async()=>{expect(hardware?.()).toBe(true);});expect(onPracticeChange).toHaveBeenLastCalledWith(false);
  // The enrolled home shows its grouped entries and bottom tabs again; enrollment guidance is hidden once enrolled.
  expect(JSON.stringify(tree!.toJSON())).toContain('보안과 복구');expect(tree!.root.findAll(node=>node.props.accessibilityRole==='tab').length).toBeGreaterThan(0);
 }finally{await Renderer.act(async()=>tree?.unmount());back.mockRestore();}
});


test('normal app shows the bottom tabs on home and hides them during sensitive recovery',async()=>{
 Object.defineProperty(Platform,'OS',{value:'ios',configurable:true});mockStored=JSON.stringify(profile());let tree:Renderer.ReactTestRenderer;
 const tabs=()=>tree!.root.findAll(node=>node.props.accessibilityRole==='tab'&&typeof node.props.onPress==='function').map(node=>node.props.accessibilityLabel);
 try{await Renderer.act(async()=>{tree=Renderer.create(<App/>);});
  expect([...new Set(tabs())]).toEqual(['홈','결제','지갑','둘러보기']);
  // Backup and phone replacement live in settings, behind the gear of every tab root.
  await tap(tree!,'설정');await tap(tree!,'독립 백업 시작');expect(tabs()).toHaveLength(0);
  await tap(tree!,'등록 상태로 돌아가기');expect(tabs()).not.toHaveLength(0);
  await tap(tree!,'설정');await tap(tree!,'원래 보드에서 분실한 휴대전화 교체');expect(tabs()).toHaveLength(0);
  await tap(tree!,'등록 상태로 돌아가기');expect(tabs()).not.toHaveLength(0);
 }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W13 returns to the entered site after W15, travel, and the installed learning entry',async()=>{
 Object.defineProperty(Platform,'OS',{value:'ios',configurable:true});
 const initial=JSON.stringify(profile());mockStored=initial;
 const link=jest.spyOn(Linking,'getInitialURL').mockResolvedValue(null);
 let tree:Renderer.ReactTestRenderer;
 try {
  await Renderer.act(async()=>{tree=Renderer.create(<App/>);});
  await openTab(tree!,'둘러보기');await tap(tree!,'DApp 둘러보기');
  const site=()=>tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='HTTPS 사이트 주소');
  const address='https://example.com/docs';
  await Renderer.act(async()=>site()!.props.onChangeText(address));
  await tap(tree!,'연결 관리');
  await tap(tree!,'등록 상태로 돌아가기');
  expect(site()?.props.value).toBe(address);
  for (const label of ['여행 지도','학습 시장']) {
    await tap(tree!,label);
    expect(site()).toBeUndefined();
    if (label === '학습 시장') {
      const market = JSON.stringify(tree!.toJSON());
      expect(market).toContain('StableNet 테스트넷 · 테스트 자산만');
      expect(market).toContain('실물 자산이나 실거래가 아닙니다.');
    }
    await tap(tree!,'지갑으로 돌아가기');
    expect(site()?.props.value).toBe(address);
  }
  await tap(tree!,'사이트 범위 검토');
  expect(JSON.stringify(tree!.toJSON())).toContain('확인된 계정과 네트워크가 있어야');
  expect(mockStored).toBe(initial);
 } finally {
  await Renderer.act(async()=>tree?.unmount());
  link.mockRestore();
 }
});


test('secure route and return announce the destination heading',async()=>{
 Object.defineProperty(Platform,'OS',{value:'ios',configurable:true});mockStored=JSON.stringify(profile());
 const announce=jest.spyOn(AccessibilityInfo,'announceForAccessibility').mockImplementation(()=>{});let tree:Renderer.ReactTestRenderer;
 try{await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
  await tap(tree!,'독립 백업 시작');expect(announce).toHaveBeenLastCalledWith('독립 백업');
  await tap(tree!,'등록 상태로 돌아가기');expect(announce).toHaveBeenLastCalledWith('개인 지갑');
 }finally{await Renderer.act(async()=>tree?.unmount());announce.mockRestore();}
});

test('W06 restores the checked address on a fresh phone without prior address metadata', async () => {
  mockBackupReady=true;mockVerify='committed';
  mockStored=JSON.stringify({...profile(),verified:undefined,
    phoneReplacement:{device:mockDevice,replacement:mockOwner,generation:'1',address:mockAddress}});
  let tree:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
    await tap(tree!,'개인 기기 찾기');await tap(tree!,'교체 후 보드 상태 확인',1);
    const saved=JSON.parse(mockStored);
    expect(saved.verified).toEqual({boardFingerprint:mockDevice,address:mockAddress,network:'8283',generation:2});
    expect(saved.phoneReplacement).toBeUndefined();
  } finally {await Renderer.act(async()=>tree?.unmount());}
});


test.each([true,false])('W05 status resumes first-address storage only with a source checkpoint (%s)', async checkpoint => {
  mockBackupReady=true;
  mockStored=JSON.stringify({...profile(),verified:undefined,
    backupCheckpoint:checkpoint?{device:mockDevice,owner:mockOwner,generation:'1'}:undefined});
  let tree:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
    await tap(tree!,'개인 기기 찾기');await tap(tree!,'등록 상태 확인',1);
    expect(JSON.parse(mockStored).verified).toEqual(checkpoint ?
      {boardFingerprint:mockDevice,address:mockAddress,network:'8283',generation:1} : undefined);
  } finally {await Renderer.act(async()=>tree?.unmount());}
});

test.each(['ko','en'] as const)('W06 %s exposes single recovery labels, keyboard dismissal and distinct candidate actions',async language=>{
 const dismissal=jest.spyOn(Keyboard,'dismiss').mockImplementation(()=>{});
 mockStored=JSON.stringify({...profile(),language});
 let tree!:Renderer.ReactTestRenderer;
 const text=(ko:string,en:string)=>language==='ko'?ko:en;
 try{
  await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
  await tap(tree,text('휴대전화 분실 및 지갑 복구','Lost phone and wallet recovery'));
  const document=tree.root.findAll(node=>node.props.testID==='w06-trust-document'&&typeof node.props.onChangeText==='function')[0];
  expect(document.props.accessibilityLabel).toBe(text('독립 복구 검증자 등록서','Independent recovery verifier enrollment'));
  expect(document.props.placeholder).not.toBe(document.props.accessibilityLabel);
  for(const id of ['original','new','verifier'])expect(tree.root.findAll(node=>node.props.testID==='w06-'+id+'-fingerprint').length).toBeGreaterThan(0);
  await tap(tree,text('키보드 닫기','Dismiss keyboard'));
  expect(dismissal).toHaveBeenCalledTimes(1);
  await tap(tree,text('새 기기 찾기','Find target board'));
  const buttons=tree.root.findAll(node=>/^w06-recover-candidate-[12]$/.test(node.props.testID??'')&&typeof node.props.onPress==='function');
  expect(new Set(buttons.map(node=>node.props.accessibilityLabel)).size).toBe(2);
  expect(new Set(buttons.map(node=>node.props.testID))).toEqual(new Set(['w06-recover-candidate-1','w06-recover-candidate-2']));
 }finally{await Renderer.act(async()=>tree?.unmount());dismissal.mockRestore();}
});

test('W06 reversed same-name candidates authorize only the enrolled target after a visible authenticated match',async()=>{
 const key=(n:number)=>concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'),p256.getPublicKey(new Uint8Array(32).fill(n),false));
 const keys=[key(1),key(2),key(3)];
 const pins=keys.map(publicKey=>bytesToHex(sha256(publicKey),false));
 const stored=profile();
 stored.enrollment.boardSPKI=pins[1];
 mockStored=JSON.stringify(stored);
 mockReverse=true;
 mockBeginRecovery.mockClear();
 mockConnect.mockClear();
 let tree!:Renderer.ReactTestRenderer;
 const button=(id:string)=>tree.root.findAll(node=>node.props.testID===id&&typeof node.props.onPress==='function')[0];
 const input=async(id:string,value:string)=>Renderer.act(async()=>{tree.root.findAll(
  node=>node.props.testID===id&&typeof node.props.onChangeText==='function')[0].props.onChangeText(value);});
 const press=async(id:string)=>Renderer.act(async()=>{button(id).props.onPress();});
 try {
  await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
  await tap(tree,'휴대전화 분실 및 지갑 복구');
  await input('w06-trust-document',JSON.stringify({source:'55'.repeat(32),targetBoard:mockDevice,sourceGeneration:1,
   original:bytesToHex(keys[0],false),target:bytesToHex(keys[1],false),accessory:bytesToHex(keys[2],false)}));
  for (const [index,id] of ['original','new','verifier'].entries())await input('w06-'+id+'-fingerprint',pins[index]);
  await tap(tree,'새 기기 찾기');
  await press('w06-recover-candidate-2');
  expect(mockConnect).toHaveBeenLastCalledWith('wrong-111111');
  expect(mockBeginRecovery).not.toHaveBeenCalled();
  expect(button('w06-authenticated-restore')).toBeUndefined();
  await press('w06-recover-candidate-1');
  expect(mockConnect).toHaveBeenLastCalledWith('right-222222');
  expect(mockBeginRecovery).not.toHaveBeenCalled();
  expect(JSON.stringify(tree.toJSON())).toContain('인증된 복구 대상');
  await press('w06-authenticated-restore');
  expect(mockBeginRecovery.mock.calls).toEqual([['right-222222']]);
  expect(mockStored).not.toMatch(/wrong-111111|right-222222/);
 }finally{await Renderer.act(async()=>tree?.unmount());mockReverse=false;}
});

test('W04 Android resumes the same enrollment explicitly after permission denial and Settings',async()=>{
 Object.defineProperty(Platform,'OS',{value:'android',configurable:true});
 mockStored=JSON.stringify({...profile(),verified:undefined});
 const original=mockStored;
 let granted=false;
 const permission=jest.fn(async()=>granted);
 const settings=jest.spyOn(Linking,'openSettings').mockResolvedValue();
 mockConnect.mockClear();
 mockRegister.mockClear();
 let tree!:Renderer.ReactTestRenderer;
 try {
  await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={permission} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
  await tap(tree,'개인 기기 찾기');
  expect(mockStored).toBe(original);
  expect(mockConnect).not.toHaveBeenCalled();
  expect(mockRegister).not.toHaveBeenCalled();
  await tap(tree,'앱 권한 설정 열기');
  expect(settings).toHaveBeenCalledTimes(1);
  granted=true;
  expect(permission).toHaveBeenCalledTimes(1);
  await tap(tree,'개인 기기 검색 다시 시도');
  expect(permission).toHaveBeenCalledTimes(2);
  await tap(tree,'기기 등록 시작',1);
  expect(mockConnect.mock.calls).toEqual([['right-222222']]);
  expect(mockRegister).toHaveBeenCalledTimes(1);
  expect(JSON.parse(mockStored).enrollment).toEqual(JSON.parse(original).enrollment);
 }finally{await Renderer.act(async()=>tree?.unmount());settings.mockRestore();}
});


test.each(['ko','en'] as const)('W06 %s chooses recovery from fresh or settings without BLE, login, or writes',async language=>{
 const personal=language==='en';
 Object.defineProperty(Platform,'OS',{value:'android',configurable:true});
 const original=personal?JSON.stringify({...profile(),language}):'';
 mockStored=original;
 mockRecoveryCloudProfile=personal?JSON.stringify({api:'https://wallet.example',identity:'https://id.example',
   approval:'https://approve.example',recovery:'https://recover.example'}):'';
 mockScanOwner.mockClear();mockConnect.mockClear();mockRegister.mockClear();mockBeginRecovery.mockClear();mockRecoveryCloudRequest.mockClear();
 const permission=jest.fn(async()=>true);
 let hardware:(()=>boolean)|undefined;
 const handler=jest.spyOn(BackHandler,'addEventListener').mockImplementation((_name,callback)=>{
   hardware=()=>callback({type:'hardwareBackPress',timeStamp:0})===true;
   return {remove(){hardware=undefined;}};
 });
 let tree!:Renderer.ReactTestRenderer;
 const label=(ko:string,en:string)=>personal?en:ko;
 const has=(name:string)=>tree.root.findAll(node=>node.props.accessibilityLabel===name&&typeof node.props.onPress==='function').length>0;
 const systemBack=()=>Renderer.act(async()=>{expect(hardware?.()).toBe(true);});
 try {
   await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={permission} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
   if(personal) await tap(tree,'Settings');
   await tap(tree,label('분실 복구','Wallet recovery'));
   expect(JSON.stringify(tree.toJSON())).toContain(label('어떤 기기를 교체하나요?','Which device are you replacing?'));
   await tap(tree,label('잃어버린 휴대전화 교체','Replace a lost phone'));
   expect(JSON.stringify(tree.toJSON())).toContain(label('원래 보드','original board'));
   expect(tree.root.findAll(node=>node.props.testID==='w06-trust-document')).toHaveLength(0);
   if(!personal) expect(tree.root.findAll(node=>node.props.testID==='w06-board-enrollment-document').length).toBeGreaterThan(0);
   await systemBack();

   await tap(tree,label('분실한 보드 복원','Restore a lost board'));
   for(const input of ['w06-trust-document','w06-original-fingerprint','w06-new-fingerprint','w06-verifier-fingerprint']){
     expect(tree.root.findAll(node=>node.props.testID===input).length).toBeGreaterThan(0);
   }
   await tap(tree,label('복구 방법으로 돌아가기','Back to recovery choices'));

   await tap(tree,label('클라우드 휴대전화 복구','Cloud phone recovery'));
   if(personal){
     await tap(tree,'Authenticate existing cloud account');
     expect(has('Sign in to the cloud service')).toBe(true);
     expect(has('클라우드 지갑 생성')).toBe(false);
     await systemBack();
   }else {
     expect(JSON.stringify(tree.toJSON())).toContain('이 설치에서는 클라우드 휴대전화 복구를 사용할 수 없습니다.');
     expect(has('기존 클라우드 계정 인증')).toBe(false);
   }
   await systemBack();
   await tap(tree,label('취소','Cancel'));
   expect(has(label('분실 복구','Wallet recovery'))).toBe(true);
   expect(mockStored).toBe(original);
   expect(permission).not.toHaveBeenCalled();
   expect(mockScanOwner).not.toHaveBeenCalled();
   expect(mockConnect).not.toHaveBeenCalled();
   expect(mockRegister).not.toHaveBeenCalled();
   expect(mockBeginRecovery).not.toHaveBeenCalled();
   expect(mockRecoveryCloudRequest).not.toHaveBeenCalled();
 }finally{
   mockRecoveryCloudProfile='';
   await Renderer.act(async()=>tree?.unmount());
   handler.mockRestore();
 }
});
