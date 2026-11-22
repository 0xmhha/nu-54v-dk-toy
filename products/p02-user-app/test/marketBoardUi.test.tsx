import {secp256k1} from '@noble/curves/secp256k1';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToHex,concat,hexToBytes} from '@nu54/protocol';
import ReactTestRenderer from 'react-test-renderer';
import {MarketLab} from '../src/market/MarketLab.tsx';
import {signedCloudCheckpoint, type Approval, type Profile, type Result, type Snapshot} from '../src/market/lab.ts';

const mockTarget = (digit:string) => ({address:'0x'+digit.repeat(40),codeHash:'0x'+digit.repeat(64)});
const mockProfile:Profile = {environment:'p08-isolated-8283-v1',chainId:8283,genesis:'0x'+'1'.repeat(64),
  deploymentHash:'0x'+'2'.repeat(64),deploymentHeight:42,quoteSigner:mockTarget('7').address,
  venue:mockTarget('3'),base:mockTarget('4'),quote:mockTarget('5')};
const mockBoard={fingerprint:'0x'+'a'.repeat(64),generation:3,user:mockTarget('8').address};
let mockJournal='';
let mockReceipt:Result|undefined;
let mockFx:{profile:Profile;quote:Approval}|undefined;
let mockQuoteIssued=false;
let mockRiskSnapshot: Partial<Snapshot>|undefined;
let mockDuringWallet: (()=>void)|undefined;
const mockSign=jest.fn();
const mockSignBoard=jest.fn();
const mockReadEndpoint=jest.fn<void, [string]>();
const mockSave=jest.fn(async (_key:string,_data:string)=>{});
const mockReadBoard=jest.fn(async (_handle:string)=>mockBoard);

jest.mock('../src/specs/NativeCommerceHttp.ts',()=>({__esModule:true,default:{
  loadJournal:async()=>mockJournal,saveJournal:(name:string,data:string)=>mockSave(name,data),
}}));
jest.mock('../src/specs/NativeMarketLab.ts',()=>({__esModule:true,default:{
  deploymentPolicy:async()=> 'loopback',
  sign:(...args: unknown[])=>mockSign(...args),
  walletAddress:async()=>mockBoard.user,
  exchange:async (path:string)=>{
    mockReadEndpoint(path);
    if (path==='/v1/quotes' && mockFx) {
      mockQuoteIssued=true;
      return JSON.stringify(mockFx.quote);
    }
    if (path.startsWith('/v1/wallet/') && mockDuringWallet) {
      mockDuringWallet();
      return JSON.stringify({nonce:0,baseBalance:'0',quoteBalance:'12000000',shares:'0',
        gasPrice:'1',gasBalance:'2000000',quoteAllowance:'12000000'});
    }
    return JSON.stringify(path==='/v1/profile' ?
      {profile:mockFx?.profile??mockProfile,state:'ready',time:mockFx?(mockQuoteIssued?100:99):1,price:'1200000',
        priceVersion:'1',observations:[{price:'1200000',observedAt:'1'},{price:'1200000',observedAt:'1'}],...mockRiskSnapshot} :
      path.startsWith('/v1/approvals/') ? mockReceipt : {baseBalance:'0',quoteBalance:'0',shares:'0',gasPrice:'1'});
  },
}}));
jest.mock('../src/market/board.ts',()=>({__esModule:true,
  scanLabBoards:async()=>['owned-board'],
  signLabBoard:(...args: unknown[])=>mockSignBoard(...args),
  readLabBoard:(handle:string)=>mockReadBoard(handle),
}));

test('selects a registered board account from the ordinary market chooser without creating a local wallet',async()=>{
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);
    });
    const control=(label:string,property:string)=>view!.root.findAll(node=>
      node.props.accessibilityLabel===label && typeof node.props[property]==='function')[0];
    const press=async (label:string)=>ReactTestRenderer.act(async()=>{control(label,'onPress').props.onPress();});
    await ReactTestRenderer.act(async()=>{
      control('학습 배포 블록 해시','onChangeText').props.onChangeText(mockProfile.deploymentHash);
    });
    await press('격리된 하드월렛 선택');
    await press('보드 선택 취소');
    expect(mockReadBoard).not.toHaveBeenCalled();
    await press('격리된 하드월렛 선택');
    await press('하드월렛 보드 후보 1 확인');
    expect(mockReadBoard).toHaveBeenCalledWith('owned-board');
    expect(mockSave).toHaveBeenCalledTimes(1);
    expect(JSON.parse(mockSave.mock.calls[0][1])).toEqual({user:mockBoard.user,profile:mockProfile,positions:[],board:mockBoard});
    expect(JSON.stringify(view!.toJSON())).toContain('하드월렛 학습 계정 · 독립 화면과 기기 승인 필요');
  } finally {
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});


test('opens a saved receipt separately from a new order after restart',async()=>{
  const hash='0x'+'4'.repeat(64);
  mockJournal=JSON.stringify({user:mockBoard.user,profile:mockProfile,positions:[],board:mockBoard,
    receipt:{state:'confirmed',txHash:hash,blockHash:'0x'+'5'.repeat(64),block:42,gasFee:'1'}});
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);
    });
    const click=async (label:string)=>ReactTestRenderer.act(async()=>{
      view!.root.findAll(node=>node.props.accessibilityLabel===label && typeof node.props.onPress==='function')[0].props.onPress();
    });
    expect(JSON.stringify(view!.toJSON())).not.toContain(hash);
    await click('최근 거래 영수증 보기');
    expect(JSON.stringify(view!.toJSON())).toContain(hash);
    expect(JSON.stringify(view!.toJSON())).not.toContain('견적 받기');
    await click('시장 주문으로 돌아가기');
    expect(JSON.stringify(view!.toJSON())).not.toContain(hash);
  } finally {
    mockJournal='';
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});


test('keeps the confirmed receipt available when opening the next order',async()=>{
  const checkpoint=signedCloudCheckpoint('0x01');
  mockReceipt={state:'confirmed',txHash:checkpoint.hash,blockHash:'0x'+'5'.repeat(64),block:42,gasFee:'1'};
  mockJournal=JSON.stringify({user:mockBoard.user,profile:mockProfile,positions:[],board:mockBoard,pending:checkpoint});
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);
    });
    const click=async (label:string)=>ReactTestRenderer.act(async()=>{
      view!.root.findAll(node=>node.props.accessibilityLabel===label && typeof node.props.onPress==='function')[0].props.onPress();
    });
    await click('원래 거래 확정 확인');
    expect(JSON.stringify(view!.toJSON())).toContain(checkpoint.hash);
    await click('유동성');
    expect(JSON.stringify(view!.toJSON())).not.toContain(checkpoint.hash);
    await click('최근 거래 영수증 보기');
    expect(JSON.stringify(view!.toJSON())).toContain(checkpoint.hash);
  } finally {
    mockJournal='';
    mockReceipt=undefined;
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});


function signedMarketQuote(order: Pick<Approval,'action'|'a'|'b'|'minA'|'minB'|'first'|'second'|'observedAt'|'deadline'>, actionIndex: bigint) {
  const word=(n:bigint)=>bigToBytes(n,32);
  const textHash=(text:string)=>keccak_256(Uint8Array.from(text,char=>char.charCodeAt(0)));
  const hex=(value:Uint8Array)=>bytesToHex(value,true);
  const secret=word(1n);
  const signer=hex(keccak_256(secp256k1.getPublicKey(secret,false).slice(1)).slice(12));
  const profile={...mockProfile,quoteSigner:signer};
  const quoteType=textHash('Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)');
  const domainType=textHash('EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)');
  const terms=concat(...[BigInt(mockBoard.user),actionIndex,BigInt(order.a),BigInt(order.b),BigInt(order.minA),BigInt(order.minB),1n,7n,BigInt(order.deadline)].map(word));
  const domain=keccak_256(concat(domainType,textHash('NU54 Isolated Market'),textHash('1'),word(8283n),
    word(BigInt(profile.venue.address)),hexToBytes(profile.venue.codeHash.slice(2))));
  const digest=keccak_256(concat(Uint8Array.of(25,1),domain,keccak_256(concat(quoteType,terms))));
  const signature=secp256k1.sign(digest,secret,{prehash:false,lowS:true});
  const selector=textHash('execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)').slice(0,4);
  const calldata=concat(selector,terms,word(320n),word(65n),signature.toCompactRawBytes(),Uint8Array.of(signature.recovery+27),new Uint8Array(31));
  return {profile,quote:{...order,user:mockBoard.user,priceVersion:'1',price:'1200000',
    digest:hex(digest),to:profile.venue.address,data:hex(calldata),nonce:'7'}};
}

test('validates a legitimately signed quote when the head advances between profile and POST',async()=>{
  mockFx=signedMarketQuote({action:'fx-base',a:'200000000',b:'0',minA:'2000000',minB:'0',
    observedAt:99,deadline:160,first:'2400000',second:'0'},4n);
  const {profile} = mockFx;
  mockJournal=JSON.stringify({user:mockBoard.user,profile,positions:[],board:mockBoard});
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);});
    const control=(label:string,property:string)=>view!.root.findAll(node=>
      node.props.accessibilityLabel===label && typeof node.props[property]==='function')[0];
    const click=async (label:string)=>ReactTestRenderer.act(async()=>{control(label,'onPress').props.onPress();});
    await click('가격과 잔액 확인');
    await click('모의 환전');
    await ReactTestRenderer.act(async()=>{control('가상 매도 LAB','onChangeText').props.onChangeText('2');});
    await ReactTestRenderer.act(async()=>{control('최소 가상 수령 LAQ','onChangeText').props.onChangeText('2');});
    await click('견적 받기');
    expect(JSON.stringify(view!.toJSON())).toContain('2.4 LAQ');
  } finally {
    mockFx=undefined;
    mockQuoteIssued=false;
    mockJournal='';
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});


test('opens the approved LP pool and cancels both reviews without requesting a quote', async () => {
  mockJournal=JSON.stringify({user:mockBoard.user, profile:mockProfile, positions:[], board:mockBoard});
  mockReadEndpoint.mockClear();
  mockSave.mockClear();
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{
      view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);
    });
    const press=async (label:string)=>ReactTestRenderer.act(async()=>{
      view!.root.findAll(node=>node.props.accessibilityLabel===label && typeof node.props.onPress==='function')[0].props.onPress();
    });
    const ui=()=>JSON.stringify(view!.toJSON());
    await press('유동성');
    expect(ui()).toContain('테스트 풀 선택');
    expect(ui()).toContain(mockProfile.venue.address);
    expect(view!.root.findAll(node=>String(node.type)==='Text' && node.children.join('')==='보유 LP 0 LP')).toHaveLength(1);
    expect(ui()).not.toContain('견적 받기');
    const reads=mockReadEndpoint.mock.calls.slice();
    for (const [action, field] of [['유동성 공급','최대 지급 LAB'], ['유동성 회수','소각 지분']]) {
      await press(action);
      expect(ui()).toContain(field);
      expect(ui()).not.toContain('테스트 풀 선택');
      await press('풀로 돌아가기');
      expect(ui()).toContain(mockProfile.venue.address);
      expect(view!.root.findAll(node=>String(node.type)==='Text' && node.children.join('')==='보유 LP 0 LP')).toHaveLength(1);
      expect(ui()).not.toContain(field);
      expect(ui()).not.toContain('견적 받기');
    }
    expect(reads).toEqual([['/v1/profile'], ['/v1/wallet/'+mockBoard.user]]);
    expect(mockReadEndpoint.mock.calls).toEqual(reads);
    expect(mockSign).not.toHaveBeenCalled();
    expect(mockSignBoard).not.toHaveBeenCalled();
    expect(mockSave).not.toHaveBeenCalled();
    await press('학습 홈');
    expect(ui()).toContain('테스트 자산으로 연습하기');
    expect(ui()).not.toContain('테스트 풀 선택');
  } finally {
    mockJournal='';
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});

test('discards a position approval if prices become stale while reading the wallet',async()=>{
  mockFx=signedMarketQuote({action:'open-short',a:'12000000',b:'100000000',minA:'0',minB:'0',
    observedAt:99,deadline:159,first:'12000000',second:'100000000'},7n);
  mockRiskSnapshot={observations:[{price:'1200000',observedAt:'99'},{price:'1200000',observedAt:'99'}]};
  mockDuringWallet=()=>{};
  mockJournal=JSON.stringify({user:mockBoard.user,profile:mockFx.profile,positions:[]});
  mockSign.mockClear();
  mockSave.mockClear();
  let view:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);});
    const control=(label:string,property:string)=>view!.root.findAll(node=>
      node.props.accessibilityLabel===label && typeof node.props[property]==='function')[0];
    const click=async(label:string)=>ReactTestRenderer.act(async()=>{control(label,'onPress').props.onPress();});
    const ui=()=>JSON.stringify(view!.toJSON());
    await click('포지션');
    await click('가격과 잔액 확인');
    await click('매도 포지션 열기');
    await ReactTestRenderer.act(async()=>{control('격리 증거금 LAQ','onChangeText').props.onChangeText('12');});
    await ReactTestRenderer.act(async()=>{control('포지션 수량 LAB','onChangeText').props.onChangeText('1');});
    await click('견적 받기');
    expect(ui()).toContain('위험 조건을 확인하세요');
    mockDuringWallet=()=>{mockRiskSnapshot={...mockRiskSnapshot,state:'price-unavailable'};};
    await click('견적 확인 및 학습 지갑에서 승인');
    expect(mockSign).not.toHaveBeenCalled();
    expect(mockSave).not.toHaveBeenCalled();
    expect(ui()).toContain('신규 위험 증가가 중지됐어요');
    expect(ui()).not.toContain('견적 확인 및 학습 지갑에서 승인');
    mockDuringWallet=()=>{};
    mockRiskSnapshot={...mockRiskSnapshot,state:'ready'};
    await click('가격과 잔액 확인');
    expect(ui()).not.toContain('견적 확인 및 학습 지갑에서 승인');
    await click('견적 받기');
    expect(ui()).toContain('위험 조건을 확인하세요');
    expect(mockSign).not.toHaveBeenCalled();
    expect(mockSave).not.toHaveBeenCalled();
  } finally {
    mockFx=undefined;
    mockRiskSnapshot=undefined;
    mockDuringWallet=undefined;
    mockQuoteIssued=false;
    mockJournal='';
    await ReactTestRenderer.act(async()=>view?.unmount());
  }
});

test.each(['native-sign','staged','attempted'] as const)(
  'holds a position before relay when price changes during %s',async(crossing)=>{
    mockFx=signedMarketQuote({action:'open-short',a:'12000000',b:'100000000',minA:'0',minB:'0',
      observedAt:99,deadline:159,first:'12000000',second:'100000000'},7n);
    mockRiskSnapshot={observations:[{price:'1200000',observedAt:'99'},{price:'1200000',observedAt:'99'}]};
    mockDuringWallet=()=>{};
    mockJournal=JSON.stringify({user:mockBoard.user,profile:mockFx.profile,positions:[]});
    const stale = () => { mockRiskSnapshot={...mockRiskSnapshot,state:'price-unavailable'}; };
    mockSign.mockReset().mockImplementation(async()=>{
      if (crossing==='native-sign') stale();
      return '0x01';
    });
    mockSave.mockClear().mockImplementation(async(_key,data)=>{
      if (JSON.parse(data).pending?.phase===crossing) stale();
    });
    mockReadEndpoint.mockClear();
    let view:ReactTestRenderer.ReactTestRenderer|undefined;
    try {
      await ReactTestRenderer.act(async()=>{view=ReactTestRenderer.create(<MarketLab back={()=>{}} permission={async()=>true}/>);});
      const control=(label:string,property:string)=>view!.root.findAll(node=>
        node.props.accessibilityLabel===label && typeof node.props[property]==='function')[0];
      const click=async(label:string)=>ReactTestRenderer.act(async()=>{control(label,'onPress').props.onPress();});
      await click('포지션');
      await click('가격과 잔액 확인');
      await click('매도 포지션 열기');
      await ReactTestRenderer.act(async()=>{control('격리 증거금 LAQ','onChangeText').props.onChangeText('12');});
      await ReactTestRenderer.act(async()=>{control('포지션 수량 LAB','onChangeText').props.onChangeText('1');});
      await click('견적 받기');
      await click('견적 확인 및 학습 지갑에서 승인');
      expect(mockSign).toHaveBeenCalledTimes(1);
      expect(mockReadEndpoint).not.toHaveBeenCalledWith('/v1/transactions');
      const ui=JSON.stringify(view!.toJSON());
      expect(ui).toContain('신규 위험 증가가 중지됐어요');
      expect(ui).not.toContain('같은 서명 거래 다시 보내기');
      if (crossing==='native-sign') expect(mockSave).not.toHaveBeenCalled();
      else expect(JSON.parse(mockSave.mock.calls.at(-1)![1]).pending).toBeUndefined();
    } finally {
      mockSign.mockReset();
      mockSave.mockReset().mockImplementation(async()=>{});
      mockFx=undefined;
      mockRiskSnapshot=undefined;
      mockDuringWallet=undefined;
      mockQuoteIssued=false;
      mockJournal='';
      await ReactTestRenderer.act(async()=>view?.unmount());
    }
  },
);
