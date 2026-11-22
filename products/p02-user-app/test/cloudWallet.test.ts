import {ed25519} from '@noble/curves/ed25519';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {hexToBytes, toBase64} from '@nu54/protocol';
import {
  CloudClient, checkOperation, prepareTransfer, readCloudProfile, requestID, signedTransfer,
  type CloudOperation, type PendingCloud,
} from '../src/wallet/cloud/client.ts';

const ascii=(value:string)=>Uint8Array.from(value,c=>c.charCodeAt(0));
const profileJSON=JSON.stringify({
  api:'https://wallet.example',identity:'https://wallet-id.example',
  approval:'https://wallet-approval.example',recovery:'https://wallet-recovery.example',
});
const pending:PendingCloud={
  id:'d59076cb-7d4e-4956-aef9-526fad576543',kind:'sign',
  transfer:prepareTransfer('0x1234567890123456789012345678901234567890','1000','0','1000000000','1000000'),
};
const operation=():CloudOperation=>{
  const digest=keccak_256(hexToBytes('02eb83aa36a780830f4240843b9aca008252089412345678901234567890123456789012345678908203e880c0'));
  const session:CloudOperation['session']={
    ID:pending.id,Wallet:'97921f53-35b1-441f-a205-4785fe2ec23e',Kind:'sign',Epoch:1,
    KeySession:'8021edca-485f-470c-9224-02f21bec0163',Expires:Math.floor(Date.now()/1000)+120,
    Digest:Array.from(digest),Intent:Array.from(sha256(ascii('nu54/p03/transfer/v1\0'+JSON.stringify(pending.transfer)))),
    Peers:[{ID:'a1',Role:'A',Epoch:1},{ID:'b1',Role:'B',Epoch:1}],
  };
  return {session,state:'reserved',notBefore:new Date().toISOString(),transfer:pending.transfer,acknowledged:0,
    fingerprint:Array.from(sha256(ascii('nu54/p03/ceremony/v1\0'+JSON.stringify(session))))};
};

it('validates the reviewed EIP-1559 preimage and independently recovers the wallet',()=>{
  const op=operation();
  expect(checkOperation(op,pending)).toBe(op);
  const secret=new Uint8Array(32);
  secret[31]=42;
  const sig=secp256k1.sign(Uint8Array.from(op.session.Digest),secret,{prehash:false,lowS:true});
  op.state='succeeded';
  op.signature=toBase64(sig.toDERRawBytes());
  op.recoveryId=sig.recovery;
  const account={id:op.session.Wallet,state:'active',epoch:1,publicKey:toBase64(secp256k1.getPublicKey(secret,false))};
  expect(signedTransfer(op,pending,account)).toMatch(/^0x02/);
  expect(()=>signedTransfer(op,pending,{...account,epoch:2})).toThrow('P03_RESULT');
  expect(()=>checkOperation(op,{...pending,transfer:{...pending.transfer!,valueWei:'1001'}})).toThrow('P03_INTENT');
  expect(()=>checkOperation({...op,session:{...op.session,Expires:op.session.Expires+1}},pending)).toThrow('P03_FINGERPRINT');
});

it('uses a separate approval origin and requests a PKCE-bound device challenge',async()=>{
  const request=jest.fn(async()=>{
    return JSON.stringify({status:200,body:JSON.stringify({deviceCode:'a'.repeat(40),
      verificationURL:'https://wallet-approval.example/consent',expiresIn:120})});
  });
  const open=jest.fn(async()=>undefined);
  const client=new CloudClient(readCloudProfile(profileJSON),async size=>new Uint8Array(size).fill(9),open,request);
  const flow=await client.authorize(operation(),pending);
  expect(flow.origin).toBe('https://wallet-approval.example');
  const [url,method,body,token]=(request.mock.calls as unknown as string[][])[0];
  expect([url,method,token]).toEqual(['https://wallet-approval.example/p03/device/start','POST','']);
  expect(JSON.parse(body)).toMatchObject({codeChallengeMethod:'S256',request:{action:'sign',transfer:pending.transfer}});
  expect(body).not.toContain(flow.verifier);
  expect(open).toHaveBeenCalledWith('https://wallet-approval.example/consent');
  expect(()=>readCloudProfile(JSON.stringify({...JSON.parse(profileJSON),approval:'https://wallet-id.example'}))).toThrow('P03_AUTHORITY');
  expect(requestID(new Uint8Array(16))).toBe('00000000-0000-4000-8000-000000000000');
});

it('accepts an independently signed refusal only for the matching recovery request and epoch',()=>{
  const subject='f891bd81-e10c-4bf3-b47a-6752e3d5d9c9';
  const op=operation();
  const request:PendingCloud={kind:'refresh',id:pending.id};
  const zero=Array(32).fill(0);
  op.session={...op.session,Kind:'refresh',Epoch:2,Digest:zero,Intent:zero};
  op.state='denied';op.transfer=undefined;
  op.fingerprint=Array.from(sha256(ascii('nu54/p03/ceremony/v1\0'+JSON.stringify(op.session))));
  const key=new Uint8Array(32).fill(19);
  const decision={subject,wallet:op.session.Wallet,session:op.session.ID,fingerprint:op.fingerprint,
    epoch:op.session.Epoch,vote:'652b9ead-046e-44ac-8948-1e032a8fd118',issuedAt:Math.floor(Date.now()/1000)};
  const sign=(value:typeof decision)=>toBase64(ed25519.sign(
    ascii('nu54/p03/recovery-refusal/v1\0wallet:recovery-refusal\0refresh\0'+JSON.stringify(value)),key));
  const expected={...op,refusal:{decision,signature:sign(decision),authority:toBase64(ed25519.getPublicKey(key)),
    recordedAt:new Date().toISOString()}};
  expect(checkOperation(expected,request,subject)).toBe(expected);
  expect(()=>checkOperation(op,request,subject)).toThrow('P03_REFUSAL');
  expect(()=>checkOperation({...expected,refusal:{...expected.refusal,decision:{...decision,epoch:3}}},request,subject)).toThrow('P03_REFUSAL');
  expect(()=>checkOperation(expected,request,'b9c653b4-8ae1-4f7b-893e-9221d413b4cd')).toThrow('P03_REFUSAL');
});

it('keeps a refused signing request terminal and bound to the original intent',async()=>{
  const refused={...operation(),state:'refused'};
  expect(checkOperation(refused,pending)).toBe(refused);
  expect(()=>checkOperation({...refused,signature:''},pending)).toThrow('P03_REFUSAL');
  expect(()=>checkOperation({...refused,recoveryId:0},pending)).toThrow('P03_REFUSAL');
  expect(()=>checkOperation({...refused,acknowledged:1},pending)).toThrow('P03_REFUSAL');
  expect(()=>checkOperation(refused,{...pending,id:'d8a69bd7-e01a-4581-9909-66484211c401'})).toThrow('P03_OPERATION');
  const client=new CloudClient(readCloudProfile(profileJSON),jest.fn(),jest.fn(),jest.fn());
  expect(()=>client.authorize(refused,pending)).toThrow('P03_APPROVAL');
});

it('opens registered login only when the app, coordinator and identity realm agree',async()=>{
  const person={id:'example',name:'Registered Identity',issuer:'https://provider.example'};
  const network='sepolia-native-test-v1';
  const profile=readCloudProfile(JSON.stringify({...JSON.parse(profileJSON),onboarding:{network,providers:[person]}}));
  const deviceCode='a'.repeat(43);
  const verificationURL=profile.identity+'/p03/device/verify?code='+deviceCode;
  const response=(body:unknown)=>JSON.stringify({status:200,body:JSON.stringify(body)});
  const open=jest.fn(async()=>undefined);
  const request=jest.fn(async (url:string)=>url===profile.api+'/v1/onboarding'
    ?response({network,identity:profile.identity})
    :response({onboarding:{network,provider:person},deviceCode,verificationURL,expiresIn:120}));
  const client=new CloudClient(profile,async size=>new Uint8Array(size).fill(9),open,request);

  await expect(client.login({...person,id:'unregistered'})).rejects.toThrow('P03_PROVIDER');
  expect(request).not.toHaveBeenCalled();
  request.mockResolvedValueOnce(response({network:'isolated-market-8283-v1',identity:profile.identity}));
  await expect(client.login(person)).rejects.toThrow('P03_ONBOARDING');
  expect(request).toHaveBeenCalledTimes(1);
  expect(open).not.toHaveBeenCalled();
  await expect(client.login(person)).resolves.toMatchObject({deviceCode,origin:profile.identity});
  expect(open).toHaveBeenCalledTimes(1);
  expect(open).toHaveBeenCalledWith(verificationURL);
  const wallet={id:operation().session.Wallet,state:'creating',epoch:0};
  request.mockResolvedValueOnce(response({...wallet,profile:'isolated-market-8283-v1:'+'4'.repeat(64)}));
  await expect(client.account('local.session')).rejects.toThrow('P03_ACCOUNT_PROFILE');
  request.mockResolvedValueOnce(response({...wallet,profile:network}));
  await expect(client.account('local.session')).resolves.toMatchObject(wallet);
});
