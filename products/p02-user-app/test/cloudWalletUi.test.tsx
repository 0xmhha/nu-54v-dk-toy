import ReactTestRenderer from 'react-test-renderer';
import {Keyboard, Linking, TextInput} from 'react-native';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bytesToHex, hexToBytes, toBase64 as mockBase64, utf8Encode} from '@nu54/protocol';
import {closureJournalName, journalName, prepareTransfer, readCloudProfile, type CloudOperation} from '../src/wallet/cloud/client.ts';
import {CloudWalletScreen} from '../src/wallet/cloud/CloudWalletScreen.tsx';

const fixture=readCloudProfile(JSON.stringify({api:'https://wallet.example',identity:'https://id.example',approval:'https://approve.example',recovery:'https://recover.example'}));
const realm=fixture.api+'|'+fixture.identity;
const alice={subject:'d0c1e728-336e-4cd8-9c9f-0e77bae14bbb',wallet:'74af6adc-dabb-45fd-bdec-f8b2147689db'};
const bob={subject:'bf260029-d859-4d4e-8475-de007102b08f',wallet:'78d9d842-c9eb-4f7d-aa62-de25380f6ec1'};
const lostRequest='5fce1b72-0c10-4b3b-8bfc-903f6387d4c1';
const to='0x1234567890123456789012345678901234567890';
const secretRequest={id:lostRequest,kind:'sign',transfer:prepareTransfer(to,'51339','0','1000000000','1000000')};
const aliceSlot=journalName(realm,alice.subject,alice.wallet);
const mockJournal=jest.fn(async (name:string)=>name===aliceSlot?
  JSON.stringify({realm,subject:alice.subject,wallet:alice.wallet,pending:secretRequest}):'');
const mockSave=jest.fn(async (_name?:string,_value?:string)=>undefined);
const mockPrepare=jest.fn(async()=>'public-csr-fixture');
const mockInstall=jest.fn(async(_subject:string,_roster:string)=>true);
const mockStart=jest.fn(async(_subject:string,_expires:number,_token:string)=>true);
const mockStop=jest.fn(async()=>true);
let rosterPublished=false;
let linkMissing=false;
let issueEnrollment=false;
let unboundProfile=false;
let identity=alice;
let offerMarketAccount=false;
const marketKey=secp256k1.getPublicKey(new Uint8Array(32).fill(5),false);
const marketAddress=bytesToHex(keccak_256(marketKey.subarray(1)).subarray(12),true);
const marketProfile='isolated-market-8283-v1:'+'4'.repeat(64);
let walletState='blocked';
let closeAttempts=0;
let closureRecord:{request:string;wallet:string;epoch:number;acceptedAt:string;completedAt?:string;assetMoves:number}|undefined;
let delayed:Promise<unknown>|undefined;
let signOutcome: 'denied'|'resolved'|undefined;
let signOperation:CloudOperation|undefined;
const mockRequest=jest.fn(async (url:string,method:string,body:string,token:string)=>{
  const response=(status:number,payload?:unknown)=>JSON.stringify({status,body:payload&&JSON.stringify(payload)});
  if(url===fixture.api+'/v1/onboarding') return response(200,{network:'sepolia-native-test-v1',identity:fixture.identity});
  if(url===fixture.identity+'/p03/device/start') return response(200,{deviceCode:'x'.repeat(30),verificationURL:fixture.identity+'/confirm',expiresIn:120});
  if(url===fixture.identity+'/p03/device/poll') return response(200,{state:'ready',token:'fixture.jwt.'+identity.subject,...(issueEnrollment?{enrollmentToken:'fixture.enroll.'+identity.subject}:{})});
  if(url===fixture.approval+'/p03/device/start')return response(200,{deviceCode:'y'.repeat(30),verificationURL:fixture.approval+'/confirm',expiresIn:120});
  if(url===fixture.approval+'/p03/device/poll')return response(200,signOutcome?{state:signOutcome}:{state:'ready',proof:'A'.repeat(86)+'=='});
  if(url===fixture.api+'/v1/close'&&method==='POST'){
    const {request,wallet,epoch}=JSON.parse(body);
    walletState='closed';closeAttempts++;
    closureRecord={request,wallet,epoch,acceptedAt:new Date().toISOString(),assetMoves:0};
    return response(503);
  }
  if(url===fixture.api+'/v1/close' || closureRecord&&url===fixture.api+'/v1/close/'+closureRecord.request){
    return closureRecord&&token.endsWith(bob.subject)?response(200,closureRecord):response(404);
  }
  const active=token.endsWith(alice.subject)?alice:bob;
  if(url===fixture.api+'/v1/enrollment'){linkMissing=false;return response(503);}
  if(url===fixture.api+'/v1/session')return linkMissing?response(403):response(200,{subject:active.subject,expires:Math.floor(Date.now()/1000)+300,linked:!unboundProfile});
  if(url===fixture.api+'/v1/wallet')return response(200,offerMarketAccount?
    {id:active.wallet,state:'active',epoch:1,address:marketAddress,publicKey:mockBase64(marketKey),profile:marketProfile}:
    {id:active.wallet,state:walletState,epoch:1,...(walletState==='active'?{address:marketAddress,publicKey:mockBase64(marketKey)}:{})});
  if(url===fixture.api+'/v1/credentials')return response(200,[]);
  if(url===fixture.api+'/v1/mobile/roster')return response(200,rosterPublished ?
    {state:'ready',roster:{Payload:'public roster fixture',Signature:'public proof fixture'}}:{state:'pending'});

  if(url.includes(lostRequest)){await delayed;return signOperation?response(200,signOperation):response(404);}
  return response(400);
});

jest.mock('../src/specs/NativeCommerceHttp.ts',()=>({__esModule:true,default:{
  randomBytes:async (size:number)=>mockBase64(new Uint8Array(size).fill(11)),
  loadJournal:(name:string)=>mockJournal(name), saveJournal:(name:string,value:string)=>mockSave(name,value),
}}));
jest.mock('../src/specs/NativeCloudWalletProfile.ts',()=>({__esModule:true,default:{
  request:(...args:Parameters<typeof mockRequest>)=>mockRequest(...args),
  prepareParticipant:()=>mockPrepare(),
  installParticipant:(...args:Parameters<typeof mockInstall>)=>mockInstall(...args),
  startParticipant:(...args:Parameters<typeof mockStart>)=>mockStart(...args),
  stopParticipant:()=>mockStop(),
}}));

test('only the authenticated owner reads or replays the saved request after reinstall login and account switches',async()=>{
  issueEnrollment=true;
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} back={()=>{}}/>);});
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{
      const button=action(label);
      expect(button?.props.disabled).toBe(false);
      button.props.onPress();
    });
    const visible=()=>JSON.stringify(tree!.toJSON());
    const login=async(user:typeof alice)=>{
      identity=user;
      await press('클라우드 서비스에 별도 로그인');
      expect(open).toHaveBeenLastCalledWith(fixture.identity+'/confirm');
      await press('브라우저 로그인 확인');
    };
    expect(visible()).not.toContain(to);
    expect(mockJournal).not.toHaveBeenCalled();
    await login(bob);
    expect(action('이 계정을 클라우드 지갑에 연계')).toBeUndefined();
    expect(visible()).not.toContain(to);
    expect(action('같은 요청 재조회')).toBeUndefined();
    expect(mockJournal).not.toHaveBeenCalledWith(aliceSlot);
    await press('클라우드 로그아웃');
    await login(alice);
    expect(visible()).toContain(to);
    expect(visible()).toContain('51339');
    let finish=()=>{};
    delayed=new Promise(resolve=>{finish=()=>resolve(undefined);});
    await press('같은 요청 재조회');
    expect(action('클라우드 로그아웃').props.disabled).toBe(true);
    expect(mockRequest).toHaveBeenCalledWith(fixture.api+'/v1/operations/'+lostRequest,'GET','','fixture.jwt.'+alice.subject);
    await ReactTestRenderer.act(async()=>finish());
    await press('클라우드 로그아웃');
    expect(visible()).not.toContain(to);
    await login(bob);
    expect(visible()).not.toContain(lostRequest);
    expect(action('같은 번호로 제출')).toBeUndefined();
    expect(mockRequest).not.toHaveBeenCalledWith(fixture.api+'/v1/operations','POST',expect.any(String),expect.any(String));
    expect(mockSave).not.toHaveBeenCalled();
  }finally{
    issueEnrollment=false;
    delayed=undefined;
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});


test.each([['denied','refused'],['resolved','running']] as const)(
  'reconciles browser signing outcome %s against the original request state %s',async(outcome,state)=>{
    identity=alice;
    walletState='active';
    const session:CloudOperation['session']={
      ID:lostRequest,Wallet:alice.wallet,Kind:'sign',Epoch:1,KeySession:'8021edca-485f-470c-9224-02f21bec0163',
      Expires:Math.floor(Date.now()/1000)+120,
      Digest:Array.from(keccak_256(hexToBytes('02eb83aa36a780830f4240843b9aca0082520894123456789012345678901234567890123456789082c88b80c0'))),
      Intent:Array.from(sha256(utf8Encode('nu54/p03/transfer/v1\0'+JSON.stringify(secretRequest.transfer)))),
      Peers:[{ID:'a1',Role:'A',Epoch:1},{ID:'b1',Role:'B',Epoch:1}],
    };
    signOperation={session,state:'reserved',notBefore:new Date(Date.now()-1000).toISOString(),acknowledged:0,
      fingerprint:Array.from(sha256(utf8Encode('nu54/p03/ceremony/v1\0'+JSON.stringify(session))))};
    const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
    let tree:ReactTestRenderer.ReactTestRenderer|undefined;
    try{
      await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} back={()=>{}}/>);});
      const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
      const press=async(label:string)=>ReactTestRenderer.act(async()=>{
        expect(action(label)?.props.disabled).toBe(false);
        action(label).props.onPress();
      });
      await press('클라우드 서비스에 별도 로그인');
      await press('브라우저 로그인 확인');
      await press('같은 요청 재조회');
      await press('요청 원문 독립 승인');
      signOutcome=outcome;
      signOperation={...signOperation,state};
      await press('독립 확인 결과 가져오기');
      const visible=JSON.stringify(tree!.toJSON());
      expect(visible.includes('이 원문을 거절했습니다. 서명 반환 · 0건')).toBe(state==='refused');
      expect(visible.includes('참여자 응답 대기')).toBe(state==='running');
      expect(visible).not.toContain('검증된 서명 거래');
      expect(action('요청 원문 독립 승인')).toBeUndefined();
      expect(mockRequest).not.toHaveBeenCalledWith(expect.stringMatching(/\/approval$/),'PUT',expect.anything(),expect.anything());
    }finally{
      signOutcome=undefined;
      signOperation=undefined;
      walletState='blocked';
      await ReactTestRenderer.act(async()=>tree?.unmount());
      open.mockRestore();
    }
});
 
test('registers the authenticated mobile participant using its public CSR and the approved roster',async()=>{
  identity=bob;
  rosterPublished=false;
  mockStart.mockResolvedValueOnce(false);
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    await press('휴대전화 참가자 등록 · 재확인');
    expect(mockPrepare).toHaveBeenCalledTimes(1);
    expect(mockInstall).not.toHaveBeenCalled();
    expect(mockRequest).toHaveBeenLastCalledWith(fixture.api+'/v1/mobile/roster','POST',
      JSON.stringify({csr:'public-csr-fixture'}),'fixture.jwt.'+bob.subject);
    rosterPublished=true;
    await press('휴대전화 참가자 등록 · 재확인');
    expect(mockInstall).toHaveBeenLastCalledWith(bob.subject,JSON.stringify({Payload:'public roster fixture',Signature:'public proof fixture'}));
    expect(mockStart).toHaveBeenLastCalledWith(bob.subject,expect.any(Number),'fixture.jwt.'+bob.subject);
    expect(action('휴대전화 참가자 등록 · 재확인')).toBeUndefined();
  }finally{
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});


test('recovers a lost enrollment response and awaits native shutdown before the next login',async()=>{
  identity=bob;
  issueEnrollment=true;
  linkMissing=true;
  mockStart.mockClear();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    expect(mockStart).not.toHaveBeenCalled();
    await press('이 계정을 클라우드 지갑에 연계');
    expect(mockStart).not.toHaveBeenCalled();
    await press('인증 연계 상태 재확인');
    expect(mockStart).toHaveBeenCalledTimes(1);
    let finish=()=>{};
    mockStop.mockImplementationOnce(()=>new Promise(resolve=>{finish=()=>resolve(true);}));
    await press('클라우드 로그아웃');
    expect(action('클라우드 서비스에 별도 로그인').props.disabled).toBe(true);
    await ReactTestRenderer.act(async()=>finish());
    expect(action('클라우드 서비스에 별도 로그인').props.disabled).toBe(false);
  }finally{
    issueEnrollment=false;linkMissing=false;
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});

test('revoked linked authorization stops the native participant during an active screen',async()=>{
  identity=bob;
  linkMissing=false;
  mockStop.mockClear();
  jest.useFakeTimers();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    expect(action('클라우드 로그아웃')).toBeDefined();
    const stopped=mockStop.mock.calls.length;
    linkMissing=true;
    await ReactTestRenderer.act(async()=>jest.advanceTimersByTime(15000));
    expect(action('클라우드 로그아웃')).toBeUndefined();
    expect(mockStop).toHaveBeenCalledTimes(stopped+1);
  }finally{
    await ReactTestRenderer.act(async()=>tree?.unmount());
    linkMissing=false;
    jest.useRealTimers();
    open.mockRestore();
  }
});


test('new pairwise subject can send public CSR before profile binding without enabling native wallet',async()=>{
  identity=bob;
  issueEnrollment=true;
  unboundProfile=true;
  rosterPublished=false;
  mockPrepare.mockClear();
  mockInstall.mockClear();
  mockStart.mockClear();
  mockJournal.mockClear();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    await press('휴대전화 참가자 등록 · 재확인');
    expect(mockPrepare).toHaveBeenCalledTimes(1);
    expect(mockInstall).not.toHaveBeenCalled();
    expect(mockStart).not.toHaveBeenCalled();
    expect(mockJournal).not.toHaveBeenCalled();
    expect(action('클라우드 지갑 생성')).toBeUndefined();
  }finally{
    issueEnrollment=false;unboundProfile=false;
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});


test('closure journals use the existing native allowlist and a separate hash domain',()=>{
  const operation=journalName(realm,bob.subject,bob.wallet);
  const exit=closureJournalName(realm,bob.subject,bob.wallet);
  expect(exit).toMatch(/^cloud-p03-[0-9a-f]{64}$/);
  expect(exit).not.toBe(operation);
  expect(exit).not.toBe(closureJournalName(realm,alice.subject,bob.wallet));
});

test('confirmed exit reopens the same receipt after loss, waits for completion and never restarts a closed wallet',async()=>{
  identity=bob;
  closeAttempts=0;
  const exitSlot=closureJournalName(realm,bob.subject,bob.wallet);
  const persisted=new Map<string,string>();
  const load=mockJournal.getMockImplementation()!;
  const save=mockSave.getMockImplementation()!;
  mockJournal.mockImplementation(async name=>persisted.get(name)??load(name));
  mockSave.mockImplementation(async (name?:string,value?:string)=>{if(name!==undefined)persisted.set(name,value??'');});
  mockStart.mockClear();
  mockStop.mockClear();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    expect(mockStart).toHaveBeenCalledTimes(1);
    await press('클라우드 계정 종료 검토');
    await press('서명 이용 종료 확인 및 독립 인증');
    await press('독립 확인 결과 가져오기');
    expect(closeAttempts).toBe(1);
    const stopped=mockStop.mock.calls.length;
    const journal=JSON.parse(persisted.get(exitSlot)!);
    expect(journal).toEqual({realm,subject:bob.subject,wallet:bob.wallet,epoch:1,request:closureRecord!.request,submitted:true});
    expect(JSON.stringify(tree!.toJSON())).toContain('종료 접수 · 서버 확인 대기');
    await press('같은 종료 요청 조회');
    expect(closeAttempts).toBe(1);
    expect(mockStop).toHaveBeenCalledTimes(stopped+1);
    expect(JSON.stringify(tree!.toJSON())).toContain('종료 요청을 접수했습니다 · 참여자 확인 대기');
    closureRecord!.completedAt=new Date().toISOString();
    await press('같은 종료 요청 조회');
    expect(JSON.stringify(tree!.toJSON())).toContain('종료 완료 · 참여자 확인 완료');
    expect(action('Sepolia 전송 요청')).toBeUndefined();
    expect(action('휴대전화 참가자 등록 · 재확인')).toBeUndefined();
    await press('클라우드 로그아웃');
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    expect(mockStart).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(tree!.toJSON())).toContain(journal.request);
    expect(closeAttempts).toBe(1);
  }finally{
    walletState='blocked';
    closureRecord=undefined;
    mockJournal.mockImplementation(load);
    mockSave.mockImplementation(save);
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});


test('leaves rejected isolated market binding visible in the authenticated chooser',async()=>{
  identity=bob;
  offerMarketAccount=true;
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  const binding=jest.fn(async()=>{throw new Error('different market deployment');});
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} back={()=>{}} selectMarket={binding}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    expect(action('이 클라우드 지갑을 격리 시장에 연결')).toBeUndefined();
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    await press('이 클라우드 지갑을 격리 시장에 연결');
    expect(binding).toHaveBeenCalledWith(expect.objectContaining({
      owner:bob.subject,account:expect.objectContaining({id:bob.wallet,address:marketAddress,profile:marketProfile}),
    }));
    expect(JSON.stringify(tree!.toJSON())).toContain('현재 배포와 일치하는 승인된 P03 학습 지갑을 선택하세요.');
    expect(action('이 클라우드 지갑을 격리 시장에 연결').props.disabled).toBe(false);
  } finally {
    offerMarketAccount=false;
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});

test('dismisses the transfer keyboard without clearing the input or preparing an approval',async()=>{
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  const dismiss=jest.spyOn(Keyboard,'dismiss').mockImplementation(()=>{});
  const previousIdentity=identity;
  const previousWalletState=walletState;
  identity=bob;
  walletState='active';
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} back={()=>{}}/>);
    });
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{
      tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
    });
    await press('클라우드 서비스에 별도 로그인');
    await press('브라우저 로그인 확인');
    await press('Sepolia 전송 요청');
    const input=tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='받는 주소')!;
    await ReactTestRenderer.act(async()=>{input.props.onChangeText(to);});
    const issued=mockRequest.mock.calls.length;
    await press('키보드 닫기');
    expect(dismiss).toHaveBeenCalledTimes(1);
    expect(input.props.value).toBe(to);
    expect(mockRequest.mock.calls).toHaveLength(issued);
  }finally{
    await ReactTestRenderer.act(async()=>tree?.unmount());
    identity=previousIdentity;
    walletState=previousWalletState;
    dismiss.mockRestore();
    open.mockRestore();
  }
});

test.each([undefined,'ko'] as const)('reopens the same scoped recovery receipt after the freeze response is lost (%s)',async(recoveryLanguage)=>{
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  const previousIdentity=identity;
  identity=bob;
  const id='0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b';
  const saved=new Map<string,string>();
  const slot=journalName(realm,bob.subject,bob.wallet);
  const json=(status:number,body?:unknown)=>JSON.stringify({status,body:body&&JSON.stringify(body)});
  const now=Math.floor(Date.now()/1000);
  const session={ID:id,Wallet:bob.wallet,Kind:'refresh',Epoch:2,KeySession:lostRequest,Expires:now+600,
    Digest:Array(32).fill(0),Intent:Array(32).fill(0),Peers:[
      {ID:'a2',Role:'A',Epoch:2},{ID:'b1',Role:'B',Epoch:1},{ID:'b2',Role:'B',Epoch:2},
      {ID:'c1',Role:'C',Epoch:1},{ID:'c2',Role:'C',Epoch:2},
    ]};
  const fingerprint=Array.from(sha256(utf8Encode('nu54/p03/ceremony/v1\0'+JSON.stringify(session))));
  let posts=0;
  const normal=mockRequest.getMockImplementation()!;
  mockRequest.mockImplementation(async(url,method,body,access)=>{
    if(url===fixture.api+'/v1/wallet') return json(200,{id:bob.wallet,state:posts?'blocked':'active',epoch:1,
      ...(posts?{}:{address:marketAddress,publicKey:mockBase64(marketKey)}),recoveryAvailable:!posts});
    if(url===fixture.api+'/v1/operations'&&method==='POST'){
      expect(JSON.parse(saved.get(slot)??'').pending.id).toBe(id);
      posts++;
      return json(503);
    }
    if(url===fixture.api+'/v1/operations/'+id&&posts) return json(200,{requestId:id,session,fingerprint,
      state:'running',acknowledged:4,notBefore:new Date(now*1000).toISOString()});
    return normal(url,method,body,access);
  });
  const defaultLoad=mockJournal.getMockImplementation()!;
  mockJournal.mockImplementation(async name=>saved.get(name)??'');
  mockSave.mockImplementation(async (name?:string,value?:string)=>{saved.set(name!,value!);});
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    const mount=async()=>ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} back={()=>{}} recoveryLanguage={recoveryLanguage}/>);});
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{
      const button=tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
      expect(button).toBeDefined();
      button.props.onPress();
    });
    const login=async()=>{await press('클라우드 서비스에 별도 로그인');await press('브라우저 로그인 확인');};
    await mount();
    expect(JSON.stringify(tree!.toJSON())).not.toContain('분실 휴대전화 교체 접수 · 서명 동결');
    expect(posts).toBe(0);
    await login();
    expect(posts).toBe(0);
    expect(saved.size).toBe(0);
    await press('분실 휴대전화 교체 접수 · 서명 동결');
    expect(JSON.stringify(tree!.toJSON())).toContain('동결 접수 · 서버 확인 대기');
    await ReactTestRenderer.act(async()=>tree?.unmount());
    await mount();await login();
    const rendered=JSON.stringify(tree!.toJSON());
    expect(rendered).toContain(id);
    expect(rendered).toContain('동결 접수 · 참여자 응답 4 / 5');
    expect(rendered).not.toContain('Sepolia 전송 요청');
    expect(posts).toBe(1);
  }finally{
    await ReactTestRenderer.act(async()=>tree?.unmount());
    mockJournal.mockImplementation(defaultLoad);
    mockSave.mockImplementation(async()=>undefined);
    mockRequest.mockImplementation(normal);
    identity=previousIdentity;
    open.mockRestore();
  }
});

test.each([['GET',200],['GET',404],['POST',200]] as const)('expired closure %s %s leaves the next session and saved receipt alone',async (heldMethod,status)=>{
  jest.useFakeTimers();
  identity=bob;
  walletState='blocked';
  closeAttempts=0;
  const load=mockJournal.getMockImplementation()!;
  const save=mockSave.getMockImplementation()!;
  const ordinary=mockRequest.getMockImplementation()!;
  const saved=new Map<string,string>();
  mockJournal.mockImplementation(async name=>saved.get(name)??load(name));
  mockSave.mockImplementation(async (name?:string,value?:string)=>{if(name)saved.set(name,value??'');});
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={{...fixture,deviceParticipant:true}} back={()=>{}}/>);
    });
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    const press=async(label:string)=>ReactTestRenderer.act(async()=>{action(label).props.onPress();});
    const login=async()=>{await press('클라우드 서비스에 별도 로그인');await press('브라우저 로그인 확인');};
    await login();
    await press('클라우드 계정 종료 검토');
    await press('서명 이용 종료 확인 및 독립 인증');
    let release:()=>void=()=>{};
    const later=new Promise<string>(resolve=>{
      release=()=>resolve(JSON.stringify({status,body:status===200?JSON.stringify(closureRecord):undefined}));
    });
    mockRequest.mockImplementation(async(url,method,body,token)=>{
      const result=await ordinary(url,method,body,token);
      const closePath=heldMethod==='POST' ? '/v1/close' : '/v1/close/'+closureRecord?.request;
      if(method===heldMethod&&url===fixture.api+closePath)return later;
      return result;
    });
    await press('독립 확인 결과 가져오기');
    const slot=closureJournalName(realm,bob.subject,bob.wallet);
    const journal=saved.get(slot);
    const record=closureRecord!;
    if(heldMethod==='GET')await press('같은 종료 요청 조회');
    await ReactTestRenderer.act(async()=>jest.advanceTimersByTime(301_000));
    expect(action('클라우드 서비스에 별도 로그인')).toBeDefined();
    mockRequest.mockImplementation(async(url,method,body,token)=>{
      if(url===fixture.api+'/v1/wallet'&&token.endsWith(alice.subject))return JSON.stringify({status:404});
      return ordinary(url,method,body,token);
    });
    identity=alice;
    await login();
    const stopped=mockStop.mock.calls.length;
    await ReactTestRenderer.act(async()=>release());
    expect(mockStop.mock.calls).toHaveLength(stopped);
    expect(saved.get(slot)).toBe(journal);
    const view=JSON.stringify(tree!.toJSON());
    expect(view).not.toContain(record.request);
    expect(view).not.toContain('서비스에 종료 접수 기록이 없습니다.');
    expect(action('클라우드 지갑 생성')).toBeDefined();
    identity=bob;
    await press('클라우드 로그아웃');
    await login();
    expect(JSON.stringify(tree!.toJSON())).toContain(record.request);
    expect(JSON.stringify(tree!.toJSON())).toContain('자산 이동 · 0건');
    expect(closeAttempts).toBe(1);
  }finally{
    walletState='blocked';
    closureRecord=undefined;
    mockRequest.mockImplementation(ordinary);
    mockJournal.mockImplementation(load);
    mockSave.mockImplementation(save);
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
    jest.useRealTimers();
  }
});

test('W20 starts with registered login scope and no user or wallet side effects',async()=>{
  mockRequest.mockClear();
  mockPrepare.mockClear();
  mockJournal.mockClear();
  mockSave.mockClear();
  const provider={id:'example',name:'Registered Identity',issuer:'https://oidc.example'};
  const profile=readCloudProfile(JSON.stringify({...fixture,onboarding:{network:'sepolia-native-test-v1',providers:[provider]}}));
  const back=jest.fn();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try{
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<CloudWalletScreen profile={profile} back={back}/>);});
    const snapshot=JSON.stringify(tree!.toJSON());
    expect(snapshot).toContain('Ethereum Sepolia');
    expect(snapshot).toContain('Registered Identity');
    expect(snapshot).toContain('모바일 A · 서비스 B · 독립 복구 C');
    expect(snapshot).toContain('활성 주소 0개');
    expect(snapshot).not.toContain('Google');
    expect(snapshot).not.toContain('Apple');
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockPrepare).not.toHaveBeenCalled();
    expect(mockJournal).not.toHaveBeenCalled();
    expect(mockSave).not.toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
    const action=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    await ReactTestRenderer.act(async()=>action('Registered Identity로 별도 로그인').props.onPress());
    expect(mockRequest.mock.calls[0]).toEqual([fixture.api+'/v1/onboarding','GET','','']);
    expect(JSON.parse(mockRequest.mock.calls[1][2]).request.onboarding).toEqual({network:'sepolia-native-test-v1',provider});
    expect(open).not.toHaveBeenCalled(); // The response did not confirm the registered profile.
    await ReactTestRenderer.act(async()=>action('지갑 선택').props.onPress());
    expect(back).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenCalledTimes(2);
  }finally{
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});


test('closing the recovery authentication screen before sign-in leaves the native participant untouched', async()=>{
  const mounted={...fixture,deviceParticipant:true};
  const back=jest.fn();
  mockStart.mockClear();
  mockStop.mockClear();
  mockRequest.mockClear();
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<CloudWalletScreen profile={mounted}
      recoveryLanguage="en" back={back} backLabel="Back to recovery information"/>);});
    await ReactTestRenderer.act(async()=>{
      tree!.root.findAll(node=>node.props.accessibilityLabel==='Back to recovery information'&&typeof node.props.onPress==='function')[0].props.onPress();
    });
    expect(back).toHaveBeenCalledTimes(1);
  } finally {
    await ReactTestRenderer.act(async()=>tree?.unmount());
  }
  expect(mockRequest).not.toHaveBeenCalled();
  expect(mockStart).not.toHaveBeenCalled();
  expect(mockStop).not.toHaveBeenCalled();
});

test('an unlinked account cannot enroll or create a new wallet through the recovery entry',async()=>{
  unboundProfile=true;
  issueEnrollment=true;
  mockSave.mockClear();
  mockRequest.mockClear();
  const open=jest.spyOn(Linking,'openURL').mockResolvedValue(undefined);
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<CloudWalletScreen profile={fixture} recoveryLanguage="en" back={()=>{}}/>);
    });
    const press=(label:string)=>ReactTestRenderer.act(async()=>{
      const button=tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
      expect(button).toBeDefined();
      button.props.onPress();
    });
    await press('Sign in to the cloud service');
    await press('브라우저 로그인 확인');
    const rendered=JSON.stringify(tree!.toJSON());
    expect(rendered).toContain('Sign in again with an existing linked cloud account.');
    expect(rendered).not.toContain('이 계정을 클라우드 지갑에 연계');
    expect(rendered).not.toContain('클라우드 지갑 생성');
    expect(rendered).not.toContain('Request replacement and freeze signing');
    expect(mockRequest.mock.calls.filter(([url,method])=>url.startsWith(fixture.api)&&method==='POST')).toHaveLength(0);
    expect(mockSave).not.toHaveBeenCalled();
  } finally {
    unboundProfile=false;issueEnrollment=false;
    await ReactTestRenderer.act(async()=>tree?.unmount());
    open.mockRestore();
  }
});
