import React from 'react';
import {TextInput} from 'react-native';
import Renderer from 'react-test-renderer';
import Browser from '../src/specs/NativeDappBrowser';
import {DappBrowserScreen} from '../src/wallet/dapp/DappBrowserScreen';

jest.mock('../src/specs/NativeOwnerWallet',()=>({__esModule:true,default:{randomBytes:jest.fn(async ()=>'IiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiIiI=')}}));
jest.mock('../src/specs/NativeDappBrowser',()=>({__esModule:true,default:{
  onRequest:jest.fn(()=>({remove:()=>{}})),onClosed:jest.fn(()=>({remove:()=>{}})),
  open:jest.fn(async()=>{}),close:jest.fn(async()=>{}),respond:jest.fn(async()=>{}),confirm:jest.fn(),confirmAllowance:jest.fn(),
}}));

test('W13 opens its named destinations and reviews the normalized HTTPS origin before browser access',async()=>{
  const native=jest.mocked(Browser!);
  native.open.mockClear();
  const verify=jest.fn(async()=>{}), external=jest.fn(), manage=jest.fn(), travel=jest.fn(), market=jest.fn();
  let rendered:Renderer.ReactTestRenderer|undefined;
  const tap=(label:string)=>Renderer.act(async()=>{
    const button=rendered!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    expect(button).toBeDefined();
    button.props.onPress();
  });
  try {
    await Renderer.act(async()=>{rendered=Renderer.create(<DappBrowserScreen
      scope={{board:'aa'.repeat(32),account:'0x'+'11'.repeat(20),generation:1,chain:11155111,genesis:'0x'+'bb'.repeat(32)}}
      boards={[{name:'Board',address:'fixture'}]} scan={()=>{}} verify={verify} back={()=>{}} abortLogin={()=>{}}
      openExternal={external} openConnections={manage} openTravel={travel} openMarket={market}/>);});
    await tap('WalletConnect');await tap('연결 관리');await tap('여행 지도');await tap('학습 시장');
    for (const go of [external,manage,travel,market]) expect(go).toHaveBeenCalledTimes(1);
    await Renderer.act(async()=>rendered!.root.findByType(TextInput).props.onChangeText('https://Example.COM:443/one?q=two'));
    await tap('사이트 범위 검토');
    expect(JSON.stringify(rendered!.toJSON())).toContain('https://example.com');
    expect(native.open).not.toHaveBeenCalled();
    expect(verify).not.toHaveBeenCalled();
    await tap('계정 공개 동의 및 사이트 열기');
    expect(verify).toHaveBeenCalledTimes(1);
    expect(native.open).toHaveBeenCalledTimes(1);
  } finally {await Renderer.act(async()=>rendered?.unmount());}
});

test('W26 real browser requests refuse NFT and multicall before confirmation and leave native send uncalled',async()=>{
  jest.clearAllMocks();
  const native=jest.mocked(Browser!);
  const owner='0x'+'11'.repeat(20), contract='0x'+'33'.repeat(20);
  const origin='https://qa.example';
  const document='55555555-5555-5555-5555-555555555555';
  const prepare=jest.fn(async()=>({wire:'unexpected',fee:1n}));
  const sign=jest.fn(async()=>'unexpected');
  const submit=jest.fn(async()=>{});
  let rendered:Renderer.ReactTestRenderer|undefined;
  const press=(label:string)=>Renderer.act(async()=>{
    rendered!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  try {
    await Renderer.act(async()=>{rendered=Renderer.create(<DappBrowserScreen
      scope={{board:'aa'.repeat(32),account:owner,generation:1,chain:11155111,genesis:'0x'+'bb'.repeat(32)}}
      boards={[{name:'Board',address:'fixture'}]} scan={()=>{}} verify={async()=>{}} back={()=>{}}
      prepareSpend={prepare} signSpend={sign} submitSpend={submit} abortLogin={()=>{}}/>);});
    await Renderer.act(async()=>{rendered!.root.findByType(TextInput).props.onChangeText(origin);});
    await press('사이트 범위 검토');
    await press('이 사이트에서 송금 요청 허용');
    await press('계정 공개 동의 및 사이트 열기');
    const session=native.open.mock.calls[0][1];
    const incoming=native.onRequest.mock.calls[0][0];
    for (const [id,selector,label] of [[1,'42842e0e','NFT 전송 요청을 거절했습니다.'],
      [2,'ac9650d8','복합 호출 요청을 거절했습니다.']] as const) {
      await Renderer.act(async()=>incoming(JSON.stringify({session,origin,document,request:JSON.stringify({
        id,method:'eth_sendTransaction',params:[{from:owner,to:contract,value:'0x0',data:'0x'+selector+'00'.repeat(96)}]})})));
      expect(JSON.stringify(rendered!.toJSON())).toContain(label+' 서명 0건 · 제출 0건');
      expect(native.respond).toHaveBeenLastCalledWith(session,document,expect.stringContaining('"code":4200'));
    }
    expect(native.confirm).not.toHaveBeenCalled();
    expect(prepare).not.toHaveBeenCalled();
    expect(sign).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
  } finally {await Renderer.act(async()=>rendered?.unmount());}
});

test('W14 browser reviews the exact login message and returns only an explicitly approved signature',async()=>{
  jest.clearAllMocks();
  const native=jest.mocked(Browser!);
  native.confirmAllowance.mockReset().mockResolvedValueOnce('reject').mockResolvedValueOnce('approve');
  const owner='0x'+'11'.repeat(20), origin='https://qa.example';
  const original='qa.example wants you to sign in with your Ethereum account:\n'+owner+'\nNonce: 8abc0023';
  const proof={issued:{wire:'fixture'},nonce:'8abc0023',expires:Math.floor(Date.now()/1000)+90};
  const sign=jest.fn(async()=> '0x'+'bb'.repeat(65));
  let rendered:Renderer.ReactTestRenderer|undefined;
  const press=(label:string)=>Renderer.act(async()=>{
    rendered!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  try {
    await Renderer.act(async()=>{rendered=Renderer.create(<DappBrowserScreen
      scope={{board:'aa'.repeat(32),account:owner,generation:1,chain:11155111,genesis:'0x'+'bb'.repeat(32)}}
      boards={[{name:'Board',address:'fixture'}]} scan={()=>{}} verify={async()=>{}} back={()=>{}} abortLogin={()=>{}}
      loginOrigin={origin} prepareLogin={async()=>proof} signLogin={sign}/>);});
    await Renderer.act(async()=>rendered!.root.findByType(TextInput).props.onChangeText(origin));
    await press('사이트 범위 검토'); await press('계정 공개 동의 및 사이트 열기');
    const session=native.open.mock.calls[0][1];
    const send=(id:number)=>Renderer.act(async()=>native.onRequest.mock.calls[0][0](JSON.stringify({session,origin,
      document:'55555555-5555-5555-5555-555555555555',request:JSON.stringify({id,method:'personal_sign',params:[original,owner]})})));
    await send(1);
    expect(native.confirmAllowance).toHaveBeenCalledWith(session,expect.any(String),expect.stringContaining(origin),expect.stringContaining(original));
    expect(sign).not.toHaveBeenCalled();
    await send(2);
    expect(sign).toHaveBeenCalledTimes(1);
    expect(native.respond).toHaveBeenLastCalledWith(session,expect.any(String),expect.stringContaining('"result":"0x'));
    expect(JSON.stringify(rendered!.toJSON())).toContain('사이트 로그인 서명을 반환했습니다');
  } finally {await Renderer.act(async()=>rendered?.unmount());}
});

test('W14 browser displays the exact allowance and signs and submits only after both user decisions',async()=>{
  const native=jest.mocked(Browser!);
  native.open.mockClear();
  native.onRequest.mockClear();
  native.respond.mockClear();
  native.confirm.mockReset().mockResolvedValue(true);
  native.confirmAllowance.mockReset().mockResolvedValueOnce('reject').mockResolvedValueOnce('approve').mockResolvedValueOnce('connections');
  const owner='0x'+'11'.repeat(20), token='0x'+'33'.repeat(20), spender='0x'+'44'.repeat(20);
  const origin='https://qa.example', document='55555555-5555-5555-5555-555555555555';
  const allowancePolicy={token,spender,maximum:'5000000'};
  const prepare=jest.fn(async()=>({wire:'authority-verified-allowance',fee:200000000000000n,amountLabel:'5.000000 DEMO'}));
  const sign=jest.fn(async()=>'0x'+'bb'.repeat(32));
  const submit=jest.fn(async()=>{}), viewConnection=jest.fn();
  let rendered:Renderer.ReactTestRenderer|undefined;
  const press=(label:string)=>Renderer.act(async()=>{
    rendered!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  try {
    await Renderer.act(async()=>{rendered=Renderer.create(<DappBrowserScreen
      scope={{board:'aa'.repeat(32),account:owner,generation:1,chain:11155111,genesis:'0x'+'bb'.repeat(32)}}
      boards={[{name:'Board',address:'fixture'}]} scan={()=>{}} verify={async()=>{}} back={()=>{}}
      loginOrigin={origin} allowancePolicy={allowancePolicy} prepareAllowance={prepare} onViewConnection={viewConnection}
      prepareSpend={jest.fn()} signSpend={sign} submitSpend={submit} abortLogin={()=>{}}/>);});
    await Renderer.act(async()=>{rendered!.root.findByType(TextInput).props.onChangeText(origin);});
    await press('사이트 범위 검토');
    await press('이 사이트에서 송금 요청 허용');
    await press('계정 공개 동의 및 사이트 열기');
    const session=native.open.mock.calls[0][1];
    const incoming=native.onRequest.mock.calls[0][0];
    const request=(id:number)=>Renderer.act(async()=>incoming(JSON.stringify({session,origin,document,request:JSON.stringify({
      id,method:'eth_sendTransaction',params:[{from:owner,to:token,value:'0x0',
        data:'0x095ea7b3'+'00'.repeat(12)+spender.slice(2)+(5000000).toString(16).padStart(64,'0')}]})})));
    await request(1);
    expect(prepare).toHaveBeenCalledWith(token,spender,5000000n,origin,session,expect.any(Number));
    expect(native.confirmAllowance).toHaveBeenCalledWith(session,document,expect.stringContaining('Approve token allowance'),
      expect.stringContaining('Token '+token+'\nSpender '+spender+'\nAllowance 5.000000 DEMO\nMaximum fee 200000000000000 wei'));
    expect(sign).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
    expect(native.respond).toHaveBeenLastCalledWith(session,document,expect.stringContaining('"code":4001'));
    await request(2);
    expect(sign).toHaveBeenCalledTimes(1);
    expect(sign).toHaveBeenCalledWith('fixture','authority-verified-allowance',expect.any(AbortSignal));
    expect(submit).toHaveBeenCalledTimes(1);
    expect(native.respond).toHaveBeenLastCalledWith(session,document,expect.stringContaining('"result":"0x'+'bb'.repeat(32)+'"'));
    await request(3);
    expect(viewConnection).toHaveBeenCalledWith(origin);
    expect(native.respond).toHaveBeenLastCalledWith(session,document,expect.stringContaining('"code":4001'));
    await request(4);
    expect(sign).toHaveBeenCalledTimes(1);
    expect(submit).toHaveBeenCalledTimes(1);
    expect(native.confirmAllowance).toHaveBeenCalledTimes(3);
  } finally {await Renderer.act(async()=>rendered?.unmount());}
});

test('W13 browser expiry retires a pending approval and requires a fresh scope review',async()=>{
  jest.useFakeTimers();
  const native=jest.mocked(Browser!);
  native.open.mockClear();
  native.onRequest.mockClear();
  native.respond.mockClear();
  native.close.mockClear();
  let approve!:(_value:boolean)=>void;
  native.confirm.mockReset().mockImplementation(()=>new Promise(resolve=>{approve=resolve;}));
  const owner='0x'+'11'.repeat(20), origin='https://qa.example';
  const prepare=jest.fn(async()=>({wire:'verified-draft',fee:1n}));
  const sign=jest.fn(async()=>'unexpected');
  const submit=jest.fn(async()=>{});
  let rendered:Renderer.ReactTestRenderer|undefined;
  const press=(label:string)=>Renderer.act(async()=>{
    rendered!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  const screen=()=><DappBrowserScreen
      scope={{board:'aa'.repeat(32),account:owner,generation:1,chain:11155111,genesis:'0x'+'bb'.repeat(32)}}
      boards={[{name:'Board',address:'fixture'}]} scan={()=>{}} verify={async()=>{}} back={()=>{}}
      prepareSpend={prepare} signSpend={sign} submitSpend={submit} abortLogin={()=>{}}/>;
  try {
    await Renderer.act(async()=>{rendered=Renderer.create(screen());});
    await Renderer.act(async()=>{rendered!.root.findByType(TextInput).props.onChangeText(origin);});
    await press('사이트 범위 검토');
    await press('이 사이트에서 송금 요청 허용');
    await press('계정 공개 동의 및 사이트 열기');
    const session=native.open.mock.calls[0][1], incoming=native.onRequest.mock.calls[0][0];
    const send=(id:number)=>incoming(JSON.stringify({session,origin,document:'55555555-5555-5555-5555-555555555555',
      request:JSON.stringify({id,method:'eth_sendTransaction',params:[{from:owner,to:'0x'+'33'.repeat(20),value:'0x1'}]})}));
    await Renderer.act(async()=>send(1));
    expect(native.confirm).toHaveBeenCalledTimes(1);
    await Renderer.act(async()=>{rendered!.update(screen());});
    expect(native.onRequest).toHaveBeenCalledTimes(1);
    expect(native.open).toHaveBeenCalledTimes(1);
    expect(native.close).not.toHaveBeenCalled();
    await Renderer.act(async()=>{jest.advanceTimersByTime(600_000);});
    await Renderer.act(async()=>{approve(true);send(2);});
    expect(sign).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
    expect(native.respond).not.toHaveBeenCalled();
    await Renderer.act(async()=>{rendered!.update(screen());});
    expect(native.onRequest).toHaveBeenCalledTimes(1);
    expect(native.open).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(rendered!.toJSON())).toContain('기존 연결로 요청할 수 없습니다');
    expect(rendered!.root.findAll(node=>node.props.accessibilityLabel==='계정 공개 동의 및 사이트 열기')).toHaveLength(0);
    await press('새 연결 검토');
    expect(native.open).toHaveBeenCalledTimes(1);
    await press('계정 공개 동의 및 사이트 열기');
    expect(native.open).toHaveBeenCalledTimes(2);
  } finally {
    await Renderer.act(async()=>rendered?.unmount());
    jest.useRealTimers();
  }
});
