import React from 'react';
import Renderer from 'react-test-renderer';
import {AccessibilityInfo, AppState, Linking, Share, StyleSheet, Text, TextInput, type AppStateStatus} from 'react-native';
const mockPermission = jest.fn(async (_ask:boolean)=>false);
jest.mock('../src/specs/NativeSubscriptionAlerts.ts',()=>({__esModule:true,default:{permission:(ask:boolean)=>mockPermission(ask)}}));
import {WalletSettings} from '../src/wallet/WalletSettings';
import {stableTestUsdc} from '../src/wallet/walletEducationRate';

test('diagnostics require an explicit preview and only share reviewed non-identifying fields', async () => {
  const share = jest.spyOn(Share,'share').mockResolvedValue({action:Share.dismissedAction,activityType:undefined});
  let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}} pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}}/>);});
  const tap=async (label:string)=>{await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel===label && node.props.onPress).props.onPress();});};
  expect(share).not.toHaveBeenCalled();
  await tap('Review diagnostics');
  expect(JSON.stringify(tree!.toJSON())).toContain('No addresses, balances, PINs or keys');
  expect(JSON.stringify(tree!.toJSON())).toContain('Verified firmware version: unavailable');
  expect(JSON.stringify(tree!.toJSON())).toContain('Last error code: unavailable');
  await tap('Share reviewed diagnostics');
  expect(share).toHaveBeenCalledTimes(1);
  const message=String(share.mock.calls[0][0].message);
  expect(message).not.toMatch(/0x|PIN|private key/i);
  share.mockRestore();
});
test('notification controls open the OS settings only on request', async()=>{
  const open=jest.spyOn(Linking,'openSettings').mockResolvedValue(undefined);
  let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="ko" setLanguage={()=>{}} back={()=>{}} practice={()=>{}} pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}}/>);});
  expect(open).not.toHaveBeenCalled();
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='기기 설정 열기' && node.props.onPress).props.onPress();});
  expect(open).toHaveBeenCalledTimes(1); open.mockRestore();
});

test('pending drafts keep their owner route and policy changes stay blocked', async()=>{
  const pendingEvm=jest.fn(), pendingChain=jest.fn(), pendingOrders=jest.fn();
  let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}}
    pendingOrders={pendingOrders} pendingEvm={pendingEvm} pendingChain={pendingChain} device={()=>{}}
    pending={{evm:true,chain:true,signing:true}}/>);});
  const ui=JSON.stringify(tree!.toJSON());
  expect(ui).toContain('Policy changes are unavailable');
  expect(ui).toContain('original device');
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Review EVM transfer' && node.props.onPress).props.onPress();});
  expect(pendingEvm).toHaveBeenCalledTimes(1); expect(pendingChain).not.toHaveBeenCalled();
});

test('notification authorization is read without prompting and refreshed on return', async()=>{
  let onState:((state:AppStateStatus)=>void)|undefined;
  const listener=jest.spyOn(AppState,'addEventListener').mockImplementation((_event,callback)=>{onState=callback;return {remove:()=>{}};});
  mockPermission.mockClear();
  let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}} pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}}/>);});
  expect(mockPermission).toHaveBeenCalledWith(false);
  await Renderer.act(async()=>{onState?.('active');});
  expect(mockPermission).toHaveBeenCalledTimes(2);
  expect(mockPermission).not.toHaveBeenCalledWith(true);
  await Renderer.act(async()=>{tree!.unmount();});
  listener.mockRestore();
});

test('fresh policy requires reviewed asset, field review and explicit independent-display action',async()=>{
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const initialize=jest.fn(); let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}}
    pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}} policyFresh
    freshAsset={{chain:'8283',address:'0x'+'12'.repeat(20),decimals:6}} initialize={initialize}/>);});
  const input=(label:string)=>tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel===label)!;
  await Renderer.act(async()=>{input('Allowed address').props.onChangeText('0x'+'34'.repeat(20));input('Per-transfer limit').props.onChangeText('10');input('Daily limit').props.onChangeText('30');});
  expect(initialize).not.toHaveBeenCalled();
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Review setup' && node.props.onPress).props.onPress();});
  expect(initialize).not.toHaveBeenCalled();
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Confirm on independent display' && node.props.onPress).props.onPress();});
  expect(initialize).toHaveBeenCalledWith('0x'+'34'.repeat(20),'10','30');
  await Renderer.act(async()=>{tree!.unmount();}); listener.mockRestore();
});

test('a read policy stays in force until explicitly reviewed and sent to the independent display',async()=>{
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const change=jest.fn();let tree:Renderer.ReactTestRenderer;
  const policy={board:'ab'.repeat(32),generation:7n,revision:1n,chain:8283n,asset:stableTestUsdc,
    destination:'0x'+'34'.repeat(20),perTransfer:10000000n,daily:30000000n,spent:0n};
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}}
    pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}} policy={policy} policyDecimals={6} change={change}/>);});
  const input=tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='New per-transfer limit')!;
  expect(JSON.stringify(tree!.toJSON())).toContain('TestUSDC');
  expect(input.props.value).toBe('10');
  await Renderer.act(async()=>{input.props.onChangeText('12');});
  expect(change).not.toHaveBeenCalled();
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Review change' && node.props.onPress).props.onPress();});
  expect(change).not.toHaveBeenCalled();
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Cancel change review' && node.props.onPress).props.onPress();});
  expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='Request change on independent display')).toHaveLength(0);
  expect(tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='New per-transfer limit')!.props.value).toBe('10');
  expect(change).not.toHaveBeenCalled();
  await Renderer.act(async()=>{input.props.onChangeText('12');});
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Review change' && node.props.onPress).props.onPress();});
  await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Request change on independent display' && node.props.onPress).props.onPress();});
  expect(change).toHaveBeenCalledWith(policy.destination,'12','30');
  expect(policy.perTransfer).toBe(10000000n);
  await Renderer.act(async()=>{tree!.unmount();});listener.mockRestore();
});


test('finality setup, recovery and reconciliation expose only their supported paths',async()=>{
 const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}}),enroll=jest.fn(),resume=jest.fn(),reconcile=jest.fn(),change=jest.fn();
 const base={language:'en' as const,setLanguage:()=>{},back:()=>{},practice:()=>{},pendingOrders:()=>{},pendingEvm:()=>{},pendingChain:()=>{},device:()=>{}};
 let tree:Renderer.ReactTestRenderer;
 await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings {...base} policyFinalityFresh finalityEnroll={enroll}/>);});
 await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Choose issuer setup file'&&node.props.onPress).props.onPress();});expect(enroll).toHaveBeenCalledTimes(1);
 await Renderer.act(async()=>{tree!.update(<WalletSettings {...base} policyFinalityPending finalityResume={resume}/>);});
 await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Resume approved setup'&&node.props.onPress).props.onPress();});expect(resume).toHaveBeenCalledTimes(1);
 const policy={board:'ab'.repeat(32),generation:7n,revision:1n,chain:8283n,asset:'0x'+'12'.repeat(20),destination:'0x'+'34'.repeat(20),perTransfer:10n,daily:30n,spent:9n,mode:'finality' as const};
 await Renderer.act(async()=>{tree!.update(<WalletSettings {...base} policy={policy} policyDecimals={6} change={change} finalityReconcile={reconcile}/>);});
 expect(JSON.stringify(tree!.toJSON())).toContain('cannot currently be changed');
 expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='Review change')).toHaveLength(0);
 await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Choose finalized evidence'&&node.props.onPress).props.onPress();});expect(reconcile).toHaveBeenCalledTimes(1);expect(change).not.toHaveBeenCalled();
 await Renderer.act(async()=>{tree!.unmount();});listener.mockRestore();
});

test('each active and pending limit includes the scope-verified asset unit', async()=>{
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const base={language:'en' as const,setLanguage:()=>{},back:()=>{},practice:()=>{},pendingOrders:()=>{},
    pendingEvm:()=>{},pendingChain:()=>{},device:()=>{}};
  const destination='0x'+'34'.repeat(20);
  const policy={board:'ab'.repeat(32),generation:7n,revision:1n,chain:8283n,asset:stableTestUsdc,
    destination,perTransfer:10000000n,daily:30000000n,spent:0n,
    pending:{perTransfer:12000000n,daily:30000000n,notBefore:173400n,destination}};
  let tree:Renderer.ReactTestRenderer;
  await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings {...base} policy={policy} policyDecimals={6}/>);});
  const contents=()=>JSON.stringify(tree!.toJSON()).replace(/","/g,'');
  expect(contents()).toContain('10.00 TestUSDC');
  expect(contents()).toContain('12.00 TestUSDC');
  expect(contents()).toContain('30.00 TestUSDC');
  await Renderer.act(async()=>{tree!.update(<WalletSettings {...base} policy={{...policy,asset:'0x'+'12'.repeat(20)}} policyDecimals={6}/>);});
  expect(contents()).not.toContain('TestUSDC');
  expect(contents()).toContain('12 ERC-20');
  await Renderer.act(async()=>{tree!.unmount();});
  listener.mockRestore();
});

test('W27 reads the full set and reviews address-book recipients and caps separately',async()=>{
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const changeSet=jest.fn();
  const alice='0x'+'33'.repeat(20),bob='0x'+'22'.repeat(20);
  const policy={board:'ab'.repeat(32),generation:7n,revision:4n,chain:8283n,asset:stableTestUsdc,
    destination:alice,recipients:[alice],perTransfer:10_000_000n,daily:30_000_000n,spent:0n};
  let tree:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings language="en" setLanguage={()=>{}} back={()=>{}} practice={()=>{}}
      pendingOrders={()=>{}} pendingEvm={()=>{}} pendingChain={()=>{}} device={()=>{}} policy={policy} policyDecimals={6}
      changeSet={changeSet} policyContacts={[{address:bob,name:'Bob'}]}/>);});
    const press=(label:string)=>Renderer.act(async()=>{
      tree!.root.find(node=>node.props.accessibilityLabel===label && node.props.onPress).props.onPress();
    });
    expect(JSON.stringify(tree!.toJSON())).toContain(alice);
    expect(JSON.stringify(tree!.toJSON())).toContain('Policy revision 4');
    await press('Edit allowed recipients');
    await press('Add from contacts Bob');
    await press('Review recipient changes');
    expect(changeSet).not.toHaveBeenCalled();
    await press('Review recipients on independent display');
    expect(changeSet).toHaveBeenLastCalledWith({recipients:[bob,alice]});
    await press('Cancel policy edit');
    await press('Edit allowed recipients');
    await press('Remove '+alice);
    await press('Review recipient changes');
    await press('Review recipients on independent display');
    expect(changeSet).toHaveBeenLastCalledWith({recipients:[]});
    await press('Cancel policy edit');
    await press('Edit limits only');
    await Renderer.act(async()=>{
      tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='New per-transfer limit')!.props.onChangeText('8');
    });
    await press('Review cap changes');
    expect(changeSet).toHaveBeenCalledTimes(2);
    await press('Review limits on independent display');
    expect(changeSet).toHaveBeenLastCalledWith({per:'8',daily:'30'});
    await Renderer.act(async()=>tree!.unmount());
  } finally { listener.mockRestore(); }
});

test('W27 previews 200 percent, reads the same practice in order and declines without real actions', async()=>{
  const appState=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const announce=jest.spyOn(AccessibilityInfo,'announceForAccessibility').mockImplementation(()=>{});
  const practice=jest.fn(),pending=jest.fn(),rejectPractice=jest.fn(),resetAccessibilityScroll=jest.fn();
  let tree:Renderer.ReactTestRenderer;
  const base={section:'access' as const,setLanguage:jest.fn(),back:jest.fn(),practice,pendingOrders:pending,
    pendingEvm:jest.fn(),pendingChain:jest.fn(),device:jest.fn(),rejectPractice,resetAccessibilityScroll};
  const tap=(label:string)=>Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel===label && node.props.onPress).props.onPress();});
  const ui=()=>JSON.stringify(tree!.toJSON());
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings {...base} language="en"/>);});
    await tap('Text and motion');
    await tap('Preview 200% and reduced motion');
    expect(resetAccessibilityScroll).toHaveBeenCalledTimes(3);
    const amount=tree!.root.findAllByType(Text).find(node=>node.props.testID==='w27-practice-amount')!;
    expect(StyleSheet.flatten(amount.props.style).fontSize).toBe(34);
    expect(ui()).toContain('Static board button guide');
    await tap('Use default text');
    expect(StyleSheet.flatten(tree!.root.findAllByType(Text).find(node=>node.props.testID==='w27-practice-amount')!.props.style).fontSize).toBe(17);
    await tap('Back to accessibility');
    await tap('Screen reader order');
    const rows=tree!.root.findAllByType(Text).filter(node=>node.props.testID?.startsWith('w27-read-')).map(node=>node.props.testID);
    expect(rows).toEqual(['w27-read-source','w27-read-amount','w27-read-recipient','w27-read-chain','w27-read-fee','w27-read-method','w27-read-cancel']);
    await tap('Hear again');
    await tap('Hear again');
    expect(announce).toHaveBeenCalledTimes(2);
    expect(announce.mock.calls[0]).toEqual(announce.mock.calls[1]);
    expect(announce.mock.calls[0][0]).toMatch(/2 practice coins.*Practice Café.*Practice mode.*Cancel/);
    await tap('Real approval screen example');
    expect(ui()).toContain('Requires the physical board');
    await tap('Back to screen reader order');
    await tap('Decline practice');
    expect(rejectPractice).toHaveBeenCalledTimes(1);
    expect(practice).not.toHaveBeenCalled();
    expect(pending).not.toHaveBeenCalled();
  } finally {
    if(tree!)await Renderer.act(async()=>tree.unmount());
    appState.mockRestore();
    announce.mockRestore();
  }
});

test('W27 notification example is generic and leaving it never requests authority until explicitly chosen', async()=>{
  const appState=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const settings=jest.spyOn(Linking,'openSettings').mockResolvedValue(undefined);
  const pending=jest.fn();
  mockPermission.mockClear();
  let tree:Renderer.ReactTestRenderer;
  const tap=(label:string)=>Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel===label && node.props.onPress).props.onPress();});
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings section="access" language="ko"
      setLanguage={jest.fn()} back={jest.fn()} practice={jest.fn()} pendingOrders={pending}
      pendingEvm={jest.fn()} pendingChain={jest.fn()} device={jest.fn()}/>);});
    await tap('알림 미리보기');
    const sample=tree!.root.findAllByType(Text).find(node=>node.props.testID==='w27-lock-notification')!;
    expect(sample.props.children).toBe('확인할 결제가 있습니다');
    expect(settings).not.toHaveBeenCalled();
    expect(mockPermission).not.toHaveBeenCalledWith(true);
    await tap('기기에서 알림 설정');
    expect(settings).toHaveBeenCalledTimes(1);
    await tap('미결 목록');
    expect(pending).toHaveBeenCalledTimes(1);
  } finally {
    if(tree!)await Renderer.act(async()=>tree.unmount());
    appState.mockRestore();
    settings.mockRestore();
  }
});

test.each([true,false])('W27 keeps the latest notification read when its value is %s', async(latest)=>{
  let onState:((state:AppStateStatus)=>void)|undefined;
  const listener=jest.spyOn(AppState,'addEventListener').mockImplementation((_event,callback)=>{onState=callback;return {remove:()=>{}};});
  let resolveOld:(enabled:boolean)=>void=()=>{};
  mockPermission.mockImplementationOnce(()=>new Promise<boolean>(resolve=>{resolveOld=resolve;}));
  mockPermission.mockResolvedValueOnce(latest);
  let tree:Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings section="access" language="en"
      setLanguage={jest.fn()} back={jest.fn()} practice={jest.fn()} pendingOrders={jest.fn()}
      pendingEvm={jest.fn()} pendingChain={jest.fn()} device={jest.fn()}/>);});
    await Renderer.act(async()=>{onState?.('active');});
    await Renderer.act(async()=>{resolveOld(!latest);});
    await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='Notification preview' && node.props.onPress).props.onPress();});
    const expected=latest ? 'On' : 'Scheduling unavailable';
    expect(tree!.root.findAllByType(Text).some(node=>Array.isArray(node.props.children) &&
      node.props.children.includes('Device notification permission') && node.props.children.includes(expected))).toBe(true);
  } finally {
    if(tree!)await Renderer.act(async()=>tree.unmount());
    listener.mockRestore();
  }
});

test.each([true,false])('W27 only asks on explicit selection and rereads the iOS result (%s)', async(granted)=>{
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  mockPermission.mockClear();
  mockPermission.mockResolvedValueOnce(false).mockResolvedValueOnce(granted).mockResolvedValueOnce(granted);
  const pending=jest.fn(),device=jest.fn();
  let tree:Renderer.ReactTestRenderer;
  const tap=(label:string)=>Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel===label && node.props.onPress).props.onPress();});
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletSettings section="access" language="en"
      setLanguage={jest.fn()} back={jest.fn()} practice={jest.fn()} pendingOrders={pending}
      pendingEvm={jest.fn()} pendingChain={jest.fn()} device={device}/>);});
    await tap('Notification preview');
    expect(mockPermission.mock.calls).toEqual([[false]]);
    await tap('Request notification permission');
    expect(mockPermission.mock.calls).toEqual([[false],[true],[false]]);
    expect(JSON.stringify(tree!.toJSON())).toContain(granted ? 'On' : 'Scheduling unavailable');
    await tap('Pending payment list');
    expect(pending).toHaveBeenCalledTimes(1);
    expect(device).not.toHaveBeenCalled();
  } finally {
    if(tree!)await Renderer.act(async()=>tree.unmount());
    listener.mockRestore();
  }
});
