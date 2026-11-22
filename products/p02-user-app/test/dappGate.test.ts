import {authorizeDappRequest,issueDappGrant,parseSiwe,refusedAdvancedRequest} from '../src/wallet/dapp/gate';

const owner={board:'1a'.repeat(32),account:'0x'+'11'.repeat(20),generation:3,chain:11155111,genesis:'0x'+'22'.repeat(32)};
const channel={transport:'browser' as const,session:'3b'.repeat(32),origin:'https://demo.example',topLevel:true};
const grant=()=>issueDappGrant(channel,owner,['eth_accounts','personal_sign'],1000,1060);
const issued={nonce:'8abc0023',origin:'https://demo.example',account:owner.account,chain:owner.chain,expires:1030};
const message=['demo.example wants you to sign in with your Ethereum account:',owner.account,'','','URI: https://demo.example/sign-in',
  'Version: 1','Chain ID: 11155111','Nonce: 8abc0023','Issued At: 1970-01-01T00:16:40.000Z','Expiration Time: 1970-01-01T00:17:10.000Z'].join('\n');

test('binds every request to the trusted frame, account, network and live grant',()=>{
  expect(authorizeDappRequest(grant(),channel,owner,{method:'eth_accounts',params:[]},1001)).toEqual({kind:'accounts',accounts:[owner.account]});
  for(const altered of [{...channel,origin:'https://also.example'},{...channel,topLevel:false},{...channel,session:'ff'.repeat(32)}])
    expect(()=>authorizeDappRequest(grant(),altered,owner,{method:'eth_accounts',params:[]},1001)).toThrow();
  expect(()=>authorizeDappRequest(grant(),channel,{...owner,genesis:'0x'+'ab'.repeat(32)},{method:'eth_accounts',params:[]},1001)).toThrow();
  expect(()=>authorizeDappRequest(grant(),channel,owner,{method:'eth_accounts',params:[]},1060)).toThrow();
  expect(()=>authorizeDappRequest({...grant(),revoked:true},channel,owner,{method:'eth_accounts',params:[]},1001)).toThrow();
  expect(()=>authorizeDappRequest(grant(),channel,owner,{method:'eth_sign',params:[]},1001)).toThrow();
});

test('checks a server-issued SIWE challenge against browser origin and exact network',()=>{
  const login=authorizeDappRequest(grant(),channel,owner,{method:'personal_sign',params:[message,owner.account]},1001);
  expect(login.kind).toBe('login');
  expect(parseSiwe(message,channel.origin,owner,issued,1001)).toMatchObject({nonce:issued.nonce,digest:expect.any(Uint8Array)});
  for(const altered of [message.replace('demo.example','also.example'),message.replace('/sign-in','@also.example/sign-in'),
    message.replace('11155111','8283'),message.replace('8abc0023','9abc0023'),message+'\nResources:\n- https://example.com'])
    expect(()=>parseSiwe(altered,channel.origin,owner,issued,1001)).toThrow();
  expect(()=>parseSiwe(message,channel.origin,owner,issued,1030)).toThrow();
});

test('native send is opt-in, address-bound and refuses contract calls or token approvals',()=>{
  const recipient='0x'+'44'.repeat(20);
  const request={method:'eth_sendTransaction',params:[{from:owner.account,to:recipient,value:'0x2a'}]};
  expect(()=>authorizeDappRequest(grant(),channel,owner,request,1001)).toThrow();
  const spend=issueDappGrant(channel,owner,['eth_sendTransaction'],1000,1060);
  expect(authorizeDappRequest(spend,channel,owner,request,1001)).toEqual({kind:'spend',recipient,value:42n});
  for (const change of [{data:'0x095ea7b3'},{chainId:'0x1'},{from:recipient},{value:'0x0'}])
    expect(()=>authorizeDappRequest(spend,channel,owner,{...request,params:[{...request.params[0],...change}]},1001)).toThrow();
  expect(()=>authorizeDappRequest(spend,channel,owner,request,1060)).toThrow();
});

test.each(['browser','walletconnect'] as const)('W14 %s cannot reinterpret finite or unlimited ERC20 approvals as native sends',transport=>{
  const current={...channel,transport};
  const scope=issueDappGrant(current,owner,['personal_sign','eth_sendTransaction'],1000,1060);
  const spender='55'.repeat(20);
  const token='0x'+'44'.repeat(20);
  for (const limit of ['05'.padStart(64,'0'),'ff'.repeat(32)]) {
    const transaction={from:owner.account,to:token,value:'0x0',
      data:'0x095ea7b3'+'00'.repeat(12)+spender+limit};
    for (const candidate of [transaction,{...transaction,value:'0x1'}]) {
      expect(()=>authorizeDappRequest(scope,current,owner,{method:'eth_sendTransaction',params:[candidate]},1001))
        .toThrow('DAPP_UNSUPPORTED_SPEND');
    }
  }
  expect(authorizeDappRequest(scope,current,owner,{method:'personal_sign',params:[message,owner.account]},1001))
    .toEqual({kind:'login',message});
});

test.each([['browser','42842e0e','nft'],['walletconnect','ac9650d8','batch']] as const)(
  'W26 %s classifies a real scoped request as a refusal without granting permission',(transport,selector,kind)=>{
    const active={...channel,transport};
    const permit=issueDappGrant(active,owner,['eth_accounts','eth_sendTransaction'],1000,1060);
    const request={method:'eth_sendTransaction',params:[{from:owner.account,to:'0x'+'22'.repeat(20),
      value:'0x0',data:'0x'+selector+'0'.repeat(192)}]};
    expect(refusedAdvancedRequest(permit,active,owner,request,1001)).toBe(kind);
    expect(()=>authorizeDappRequest(permit,active,owner,request,1001)).toThrow('DAPP_UNSUPPORTED_SPEND');
    expect(refusedAdvancedRequest(permit,{...active,topLevel:false},owner,request,1001)).toBeUndefined();
    expect(refusedAdvancedRequest(permit,{...active,origin:'https://different.example'},owner,request,1001)).toBeUndefined();
    expect(refusedAdvancedRequest(permit,active,{...owner,genesis:'0x'+'ab'.repeat(32)},request,1001)).toBeUndefined();
    expect(refusedAdvancedRequest(permit,active,owner,request,1060)).toBeUndefined();
  },
);

test.each(['browser','walletconnect'] as const)('W14 %s authorizes only the commissioned finite allowance',transport=>{
  const current={...channel,transport};
  const permit=issueDappGrant(current,owner,['eth_sendTransaction'],1000,1060);
  const token='0x'+'44'.repeat(20), spender='0x'+'55'.repeat(20);
  const policy={token,spender,maximum:'5000000'};
  const approve=(amount:bigint,target=spender)=>'0x095ea7b3'+'0'.repeat(24)+target.slice(2)+amount.toString(16).padStart(64,'0');
  const original={from:owner.account,to:token,value:'0x0',data:approve(5000000n)};
  const request=(tx:object)=>({method:'eth_sendTransaction',params:[tx]});
  expect(authorizeDappRequest(permit,current,owner,request(original),1001,policy))
    .toEqual({kind:'allowance',token,spender,amount:5000000n});
  for(const mutation of [{from:spender},{to:spender},{value:'0x1'},{data:approve(0n)},{data:approve(5000001n)},
    {data:approve((1n<<256n)-1n)},{data:approve(1n,token)},{data:original.data+'00'},{data:original.data.replace('095ea7b3','a9059cbb')},
    {data:original.data.replace('095ea7b30','095ea7b31')},{gas:'0x10000'},{chainId:'0x1'}]) {
    expect(()=>authorizeDappRequest(permit,current,owner,request({...original,...mutation}),1001,policy)).toThrow();
  }
  expect(()=>authorizeDappRequest(permit,{...current,topLevel:false},owner,request(original),1001,policy)).toThrow();
  expect(()=>authorizeDappRequest(permit,current,owner,request(original),1060,policy)).toThrow();
});
