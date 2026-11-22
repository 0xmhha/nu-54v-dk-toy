import {useState} from 'react';
import Renderer from 'react-test-renderer';
import {RenterReturnStatus} from '../src/wallet/RenterReturnStatus.tsx';

const mockLoad=jest.fn();
const mockExchange=jest.fn();
jest.mock('../src/specs/NativeCommerceHttp.ts',()=>({__esModule:true,default:{
  loadProfile:(slot:string)=>mockLoad(slot),exchangeRenterReturn:(...args:string[])=>mockExchange(...args),
}}));

const ref='1e08b266-5121-4676-982e-b2307dc95610';
const account='0x'+'ab'.repeat(20);
const response=()=>JSON.stringify({assignment:ref,generation:'3',account,assignmentUntil:new Date(Date.now()+60_000).toISOString(),slot:'unconfirmed',settlement:'unknown'});
function field(tree:Renderer.ReactTestRenderer,label:string) {
  return tree.root.findAll(node=>node.props.accessibilityLabel===label)[0];
}
const visible=(tree:Renderer.ReactTestRenderer)=>JSON.stringify(tree.toJSON());

test('W12 accepts the exact handover, then rereads only the original scope',async()=>{
  mockLoad.mockResolvedValue('enrolled-customer');
  mockExchange.mockResolvedValue(response());
  let tree!:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>{
      field(tree,'Rental handover reference').props.onChangeText(ref);
      field(tree,'Handed-over rental account').props.onChangeText(account);
    });
    await Renderer.act(async()=>field(tree,'Accept handover for this rental account').props.onPress());
    expect(mockExchange).toHaveBeenLastCalledWith(expect.stringMatching(/^[0-9a-f]{64}$/),ref,account);
    expect(visible(tree)).toContain(account);
    expect(visible(tree)).toContain('Original rental generation');
    expect(visible(tree)).toContain('Pending payments could not be verified.');
    mockExchange.mockRejectedValueOnce(new Error('OFFLINE'));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(mockExchange).toHaveBeenLastCalledWith(expect.any(String),ref,'');
    expect(visible(tree)).not.toContain('Original rental generation');
    expect(visible(tree)).toContain('This rental status could not be verified.');
    expect(field(tree,'Rental handover reference').props.value).toBe('');
    expect(field(tree,'Handed-over rental account').props.value).toBe('');
  } finally {await Renderer.act(async()=>tree?.unmount());}
});

test('W12 suppresses old replies with a replaced customer profile and on reopening',async()=>{
  mockLoad.mockReset();
  mockLoad.mockResolvedValue('customer-first');
  mockExchange.mockImplementationOnce(async()=>{
    mockLoad.mockResolvedValue('customer-second');
    return response();
  });
  let tree!:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).not.toContain('Original rental account');
    expect(field(tree,'Rental handover reference').props.value).toBe('');
    await Renderer.act(async()=>{tree.update(<RenterReturnStatus key="new-session" language="en" active/>);});
    expect(field(tree,'Check this rental').props.disabled).toBe(true);
    expect(visible(tree)).not.toContain(account);
  } finally {await Renderer.act(async()=>tree?.unmount());}
});


test('W12 drops identity-bound input and status on background or a changed enrolled profile',async()=>{
  jest.useFakeTimers();
  mockLoad.mockReset();mockLoad.mockResolvedValue('scope-a');
  mockExchange.mockReset();mockExchange.mockResolvedValue(response());
  let tree!:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Original rental account');
    mockLoad.mockResolvedValue('scope-b');
    await Renderer.act(async()=>{jest.advanceTimersByTime(1000);});
    expect(visible(tree)).not.toContain(account);
    expect(field(tree,'Rental handover reference').props.value).toBe('');
    await Renderer.act(async()=>field(tree,'Handed-over rental account').props.onChangeText(account));
    await Renderer.act(async()=>{tree.update(<RenterReturnStatus language="en" active={false}/>);});
    expect(field(tree,'Handed-over rental account').props.value).toBe('');
  } finally {
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});


test.each([[1,0],[0,0],[0,1],[1,1]])('W12 displays %s payments and %s limit changes and expires both',async (payments,limits)=>{
  jest.useFakeTimers();
  mockLoad.mockReset();mockLoad.mockResolvedValue('scope-count');
  mockExchange.mockReset();mockExchange.mockResolvedValue(JSON.stringify({...JSON.parse(response()),settlement:'verified',
    pendingCount:payments+limits,pendingPayments:payments,pendingLimits:limits,basisAt:new Date(Date.now()-1000).toISOString(),basisUntil:new Date(Date.now()+4000).toISOString()}));
  let tree!:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Pending assessment time');
    expect(tree.root.findAll(n=>Array.isArray(n.props.children)&&n.props.children.includes('Pending payments'))
      .some(n=>n.props.children.includes(payments))).toBe(true);
    expect(tree.root.findAll(n=>Array.isArray(n.props.children)&&n.props.children.includes('Pending limit changes'))
      .some(n=>n.props.children.includes(limits))).toBe(true);
    expect(visible(tree)).toContain(payments===0?'There are no pending payments to open.':
      'Links to the original pending payments are awaiting verification.');
    expect(tree.root.findAll(node=>node.props.accessibilityRole==='link')).toHaveLength(0);
    await Renderer.act(async()=>{jest.advanceTimersByTime(4001);});
    expect(visible(tree)).not.toContain('Pending assessment time');
    expect(visible(tree)).toContain('Pending payments could not be verified.');
    expect(visible(tree)).toContain('Original rental account');
  } finally {
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});

test('W12 does not infer zero payments from the older aggregate-only response',async()=>{
  mockLoad.mockReset();mockLoad.mockResolvedValue('scope-aggregate');
  mockExchange.mockReset();mockExchange.mockResolvedValue(JSON.stringify({...JSON.parse(response()),settlement:'verified',
    pendingCount:1,basisAt:new Date(Date.now()-1000).toISOString(),basisUntil:new Date(Date.now()+4000).toISOString()}));
  let tree!:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('This rental status could not be verified.');
    expect(visible(tree)).not.toContain('Pending assessment time');
    expect(visible(tree)).not.toContain('Pending payments');
  } finally {
    await Renderer.act(async()=>tree?.unmount());
  }
});

test.each([true,false])('W12 routes to one original payment with trusted token metadata %s',async hasMetadata=>{
  const origin='159cd798-ab43-4ee1-919a-6b7526d590a8';
  const linked={origin,amount:'20',asset:'0x'+'34'.repeat(20)};
  const verified={...JSON.parse(response()),settlement:'verified',pendingCount:2,pendingPayments:1,pendingLimits:1,
    basisAt:new Date(Date.now()-1000).toISOString(),basisUntil:new Date(Date.now()+5000).toISOString()};
  mockLoad.mockReset();mockLoad.mockResolvedValue('current-renter');
  const metadata=hasMetadata?{tokenDecimals:6,tokenSymbol:'tUSDC'}:{};
  mockExchange.mockReset().mockResolvedValue(JSON.stringify({...verified,pendingOrders:[linked],...metadata}));
  let tree!:Renderer.ReactTestRenderer;
  const label='Open original pending payment '+origin;
  function Navigate(){
    const [detail,setDetail]=useState(false);
    return <RenterReturnStatus language="en" active detailVisible={detail} navigateOriginal={setDetail}/>;
  }
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<Navigate/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(tree.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function').length).toBeGreaterThan(0);
    await Renderer.act(async()=>field(tree,label).props.onPress());
    expect(visible(tree)).toContain('Original pending payment');
    expect(visible(tree)).toContain(linked.asset);
    expect(visible(tree).includes('0.000020')).toBe(hasMetadata);
    expect(visible(tree)).toContain('Amount (base units)');
    expect(mockExchange).toHaveBeenCalledTimes(2);
    expect(mockExchange.mock.calls.every(call=>call[1]===ref&&call[2]==='')).toBe(true);
    await Renderer.act(async()=>field(tree,'Back to rental status').props.onPress());
    mockExchange.mockResolvedValue(JSON.stringify({...verified,pendingOrders:[]}));
    await Renderer.act(async()=>field(tree,label).props.onPress());
    expect(visible(tree)).not.toContain('Original pending payment');
    expect(visible(tree)).toContain('Links to the original pending payments are awaiting verification.');
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 rejects an original amount above uint256 without exposing a link',async()=>{
  const origin='159cd798-ab43-4ee1-919a-6b7526d590a8';
  mockLoad.mockReset().mockResolvedValue('current-renter');
  mockExchange.mockReset().mockResolvedValue(JSON.stringify({...JSON.parse(response()),settlement:'verified',
    pendingCount:1,pendingPayments:1,pendingLimits:0,basisAt:new Date(Date.now()-100).toISOString(),
    basisUntil:new Date(Date.now()+5000).toISOString(),pendingOrders:[{origin,amount:'9'.repeat(78),asset:'0x'+'34'.repeat(20)}]}));
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('This rental status could not be verified.');
    expect(visible(tree)).not.toContain(origin);
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test('W12 never treats handover, a pairing window, or a stale session as a rental passkey deadline',async()=>{
  mockLoad.mockReset().mockResolvedValue('same-renter');
  mockExchange.mockReset().mockResolvedValue(response());
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    expect(visible(tree)).toContain('Rental passkey validity · Unconfirmed');
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Handover confirmation expires');
    expect(visible(tree)).toContain('Ask the rental operator');
    expect(visible(tree)).toContain('Rental passkey validity · Unconfirmed');
    mockExchange.mockResolvedValue(JSON.stringify({...JSON.parse(response()),passkeyUntil:'2077-09-10T01:00:00Z',pairingWindowSeconds:120}));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('This rental status could not be verified.');
    expect(visible(tree)).not.toContain('2077');
    await Renderer.act(async()=>tree.unmount());
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    expect(visible(tree)).toContain('Rental passkey validity · Unconfirmed');
    expect(visible(tree)).not.toContain('Handover confirmation expires');
  } finally {await Renderer.act(async()=>tree?.unmount());}
});

const utcSecond=(date:number)=>new Date(Math.floor(date/1000)*1000).toISOString().replace('.000Z','Z');

test('W12 shows credential end and proof freshness separately and removes either expired claim independently',async()=>{
  jest.useFakeTimers({now:new Date('2026-11-21T00:00:00Z')});
  mockLoad.mockReset().mockResolvedValue('credential-renter');
  const now=Date.now();
  const evidenceUntil=utcSecond(now+5000), expiresAt=utcSecond(now+3_600_000);
  mockExchange.mockReset().mockResolvedValue(JSON.stringify({...JSON.parse(response()),settlement:'verified',
    pendingCount:1,pendingPayments:1,pendingLimits:0,basisAt:utcSecond(now-1000),basisUntil:utcSecond(now+3000),
    credentialExpiresAt:expiresAt,credentialEvidenceUntil:evidenceUntil}));
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Rental passkey expires');
    expect(visible(tree)).toContain(new Date(expiresAt).toLocaleString('en-US'));
    expect(visible(tree)).toContain('Passkey evidence valid until');
    expect(visible(tree)).toContain(new Date(evidenceUntil).toLocaleString('en-US'));
    await Renderer.act(async()=>{jest.advanceTimersByTime(3000);});
    expect(visible(tree)).not.toContain('Pending assessment time');
    expect(visible(tree)).toContain('Rental passkey expires');
    await Renderer.act(async()=>{jest.advanceTimersByTime(2000);});
    expect(visible(tree)).not.toContain('Rental passkey expires');
    expect(visible(tree)).toContain('Rental passkey validity · Unconfirmed');
    expect(visible(tree)).toContain('Original rental account');
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});

test.each(['missing-end','stale','beyond-assignment','beyond-term','invalid-calendar','non-utc','beyond-response-window'] as const)(
  'W12 masks %s credential evidence while keeping verified settlement',async kind=>{
    jest.useFakeTimers({now:new Date('2026-11-21T00:00:00Z')});
    const now=Date.now();
    const credential:Record<string,string>={credentialExpiresAt:utcSecond(now+100_000),credentialEvidenceUntil:utcSecond(now+5000)};
    if(kind==='missing-end')delete credential.credentialExpiresAt;
    if(kind==='stale')credential.credentialEvidenceUntil=utcSecond(now);
    if(kind==='beyond-assignment')credential.credentialEvidenceUntil=utcSecond(now+65_000);
    if(kind==='beyond-term')credential.credentialExpiresAt=utcSecond(now+3000);
    if(kind==='invalid-calendar')credential.credentialExpiresAt='2027-02-30T00:00:00Z';
    if(kind==='non-utc')credential.credentialExpiresAt='2027-01-01T00:00:00+00:00';
    if(kind==='beyond-response-window')credential.credentialEvidenceUntil=utcSecond(now+31_000);
    mockLoad.mockReset().mockResolvedValue('credential-renter');
    mockExchange.mockReset().mockResolvedValue(JSON.stringify({...JSON.parse(response()),settlement:'verified',
      pendingCount:1,pendingPayments:0,pendingLimits:1,basisAt:utcSecond(now-1000),basisUntil:utcSecond(now+10_000),...credential}));
    let tree!:Renderer.ReactTestRenderer;
    try{
      await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
      await Renderer.act(async()=>field(tree,'Rental handover reference').props.onChangeText(ref));
      await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
      expect(visible(tree)).toContain('Pending assessment time');
      expect(visible(tree)).toContain('Rental passkey validity · Unconfirmed');
      expect(visible(tree)).not.toContain('Rental passkey expires');
    }finally{
      await Renderer.act(async()=>tree?.unmount());
      jest.useRealTimers();
    }
  },
);

test('W12 uses credential evidence only on a current read and clears it after refusal or reopening',async()=>{
  jest.useFakeTimers({now:new Date('2026-11-21T00:00:00Z')});
  mockLoad.mockReset().mockResolvedValue('current-credential-profile');
  const enhanced=JSON.stringify({...JSON.parse(response()),credentialExpiresAt:utcSecond(Date.now()+60_000),
    credentialEvidenceUntil:utcSecond(Date.now()+5000)});
  mockExchange.mockReset().mockResolvedValue(enhanced);
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="ko" active/>);});
    await Renderer.act(async()=>{
      field(tree,'대여 인도 참조').props.onChangeText(ref);
      field(tree,'인도받은 대여 계정').props.onChangeText(account);
    });
    await Renderer.act(async()=>field(tree,'이 계정의 대여 인도 수락').props.onPress());
    expect(visible(tree)).toContain('대여 패스키 사용기한 · 확인 불가');
    expect(mockExchange).toHaveBeenLastCalledWith(expect.any(String),ref,account);
    await Renderer.act(async()=>field(tree,'같은 대여 상태 조회').props.onPress());
    expect(visible(tree)).toContain('패스키 근거 유효기한');
    expect(mockExchange).toHaveBeenLastCalledWith(expect.any(String),ref,'');
    mockExchange.mockRejectedValueOnce(new Error('NOT_FOUND'));
    await Renderer.act(async()=>field(tree,'같은 대여 상태 조회').props.onPress());
    expect(visible(tree)).not.toContain('패스키 근거 유효기한');
    expect(visible(tree)).not.toContain(account);
    expect(visible(tree)).toContain('대여 패스키 사용기한 · 확인 불가');
    await Renderer.act(async()=>field(tree,'대여 인도 참조').props.onChangeText(ref));
    await Renderer.act(async()=>field(tree,'같은 대여 상태 조회').props.onPress());
    expect(visible(tree)).toContain('패스키 근거 유효기한');
    await Renderer.act(async()=>tree.update(<RenterReturnStatus language="ko" active={false}/>));
    expect(visible(tree)).not.toContain('패스키 근거 유효기한');
    await Renderer.act(async()=>tree.update(<RenterReturnStatus key="fresh-process-view" language="ko" active/>));
    expect(visible(tree)).toContain('대여 패스키 사용기한 · 확인 불가');
    expect(visible(tree)).not.toContain('패스키 근거 유효기한');
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});

test('W12 renders a fresh slot observation and expires it independently of credential and settlement',async()=>{
  jest.useFakeTimers({now:new Date('2026-11-21T00:00:01Z')});
  const now=Date.now();
  const base=JSON.parse(response());
  const live={...base,slot:'open',slotCheckedAt:utcSecond(now-1000),slotEvidenceUntil:utcSecond(now+3000),
    credentialExpiresAt:utcSecond(now+60_000),credentialEvidenceUntil:utcSecond(now+5000),
    settlement:'verified',pendingCount:1,pendingPayments:0,pendingLimits:1,
    basisAt:utcSecond(now-1000),basisUntil:utcSecond(now+10_000)};
  mockLoad.mockReset().mockResolvedValue('slot-renter-profile');
  mockExchange.mockReset().mockResolvedValue(JSON.stringify(live));
  let tree!:Renderer.ReactTestRenderer;
  try{
    await Renderer.act(async()=>{tree=Renderer.create(<RenterReturnStatus language="en" active/>);});
    await Renderer.act(async()=>{
      field(tree,'Rental handover reference').props.onChangeText(ref);
      field(tree,'Handed-over rental account').props.onChangeText(account);
    });
    await Renderer.act(async()=>field(tree,'Accept handover for this rental account').props.onPress());
    expect(visible(tree)).toContain('Rental slot · Unconfirmed');
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Rental slot · Open');
    expect(visible(tree)).toContain('Slot checked at');
    expect(visible(tree)).toContain('Slot evidence valid until');
    jest.setSystemTime(now-2000);
    await Renderer.act(async()=>tree.update(<RenterReturnStatus language="ko" active/>));
    expect(visible(tree)).toContain('대여 슬롯 · 확인 불가');
    jest.setSystemTime(now);
    await Renderer.act(async()=>tree.update(<RenterReturnStatus language="en" active/>));
    expect(visible(tree)).toContain('Rental slot · Open');
    await Renderer.act(async()=>jest.advanceTimersByTime(3000));
    expect(visible(tree)).toContain('Rental slot · Unconfirmed');
    expect(visible(tree)).not.toContain('Slot checked at');
    expect(visible(tree)).toContain('Rental passkey expires');
    expect(visible(tree)).toContain('Pending limit changes');
    for(const result of [
      {...live,slotCheckedAt:utcSecond(now+4000),slotEvidenceUntil:utcSecond(now+6000)},
      {...live,slotEvidenceUntil:utcSecond(now+31_000)},
      {...live,slotCheckedAt:undefined,slotEvidenceUntil:utcSecond(now+6000)},
      {...live,slot:'unconfirmed',slotEvidenceUntil:utcSecond(now+6000)},
      base,
    ]){
      mockExchange.mockResolvedValueOnce(JSON.stringify(result));
      await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
      expect(visible(tree)).toContain('Rental slot · Unconfirmed');
      expect(visible(tree)).not.toContain('Rental slot · Open');
    }
    mockExchange.mockResolvedValueOnce(JSON.stringify({...live,slotEvidenceUntil:utcSecond(now+6000)}));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).toContain('Rental slot · Open');
    mockExchange.mockRejectedValueOnce(new Error('NOT_FOUND'));
    await Renderer.act(async()=>field(tree,'Check this rental').props.onPress());
    expect(visible(tree)).not.toContain('Rental slot · Open');
    expect(visible(tree)).not.toContain(account);
  }finally{
    await Renderer.act(async()=>tree?.unmount());
    jest.useRealTimers();
  }
});
