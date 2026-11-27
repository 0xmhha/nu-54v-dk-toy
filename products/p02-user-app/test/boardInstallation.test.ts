import {ed25519} from '@noble/curves/ed25519';
import {bytesToHex, hexToBytes, utf8Encode} from '@nu54/protocol';
import {verifyInstallation} from '../src/wallet/boardInstallation';

const secret=hexToBytes('1'.repeat(64));
const publicKey=bytesToHex(ed25519.getPublicKey(secret),false);
const claim={format:'nu54-board-install-v1',model:'nu54v_dk/nrf54l15/cpuapp',channel:'custom',candidate:'a'.repeat(64),device:'b'.repeat(64),bootKey:'c'.repeat(64),measurement:'d'.repeat(64),counter:7,generation:2,challenge:'e'.repeat(64),issuedAt:100,expiresAt:200,evidence:'observed'};
function signed(data=claim) {const payload=JSON.stringify(data); return {payload,signature:bytesToHex(ed25519.sign(utf8Encode(payload),secret),false)};}
const clock={boot:'12345678-1234-1234-1234-123456789abc',uptimeMs:100000,wallMs:150000};
const expected={publicKey,channel:'custom' as const,candidate:claim.candidate,device:claim.device,bootKey:claim.bootKey,measurement:claim.measurement,minimumCounter:7,generation:2,challenge:claim.challenge,now:150};
test('validates independently trusted actual installation and binds all replay dimensions',()=>{
 expect(verifyInstallation(signed(),expected)).toEqual(claim);
 for(const bad of [{...expected,now:200},{...expected,device:'f'.repeat(64)},{...expected,candidate:'f'.repeat(64)},{...expected,measurement:'f'.repeat(64)},{...expected,challenge:'f'.repeat(64)},{...expected,generation:3},{...expected,minimumCounter:8},{...expected,minimumCounter:NaN},{...expected,minimumCounter:-1},{...expected,now:NaN},{...expected,channel:'bogus' as 'custom'}])
  expect(()=>verifyInstallation(signed(),bad)).toThrow();
 expect(()=>verifyInstallation(signed({...claim,evidence:'synthetic'}),expected)).toThrow();
 expect(()=>verifyInstallation({...signed(),signature:'0'.repeat(128)},expected)).toThrow();
});
test('same consumed receipt resumes without permitting a different request',()=>{
 const {consumeInstallation} = require('../src/wallet/boardInstallation');
 const receipt=signed({...claim,issuedAt:150});
 const first=consumeInstallation({state:'pending',expected,clock},receipt,clock);
 expect(first.state).toBe('consumed');
 expect(consumeInstallation(first,receipt,clock)).toEqual(first);
 expect(()=>consumeInstallation(first,receipt,{...clock,boot:'22345678-1234-1234-1234-123456789abc'})).toThrow();
 expect(()=>consumeInstallation(first,receipt,{...clock,uptimeMs:150000,wallMs:200000})).toThrow();
 expect(()=>consumeInstallation(first,signed({...claim,measurement:'f'.repeat(64)}),clock)).toThrow();
});
test('preflight cannot choose its own independent attester or board',()=>{
 const {startInstallation} = require('../src/wallet/boardInstallation');
 const preflight={channel:'custom',candidate:claim.candidate,bootKey:claim.bootKey,measurement:claim.measurement,minimumCounter:7,publicKey};
 const {sha256}=require('@noble/hashes/sha256');
 const fp=bytesToHex(sha256(hexToBytes('302a300506032b6570032100'+publicKey)),false);
 expect(startInstallation(preflight,fp,claim.candidate,claim.device,0,claim.challenge,clock).expected.challenge).toBe(claim.challenge);
 expect(()=>startInstallation(preflight,'f'.repeat(64),claim.candidate,claim.device,0,claim.challenge,clock)).toThrow();
 expect(()=>startInstallation(preflight,fp,'f'.repeat(64),claim.device,0,claim.challenge,clock)).toThrow();
});
test('protected storage revalidates checkpoint after restart',()=>{
 const {consumeInstallation}=require('../src/wallet/boardInstallation');
 const {decodeWalletStorage}=require('../src/wallet/profile');
 const receipt=signed({...claim,issuedAt:150}); const checkpoint=consumeInstallation({state:'pending',expected,clock},receipt,clock);
 expect(decodeWalletStorage(JSON.stringify({version:2,exercised:false,installation:checkpoint})).installation).toEqual(checkpoint);
 const altered={...checkpoint,claim:{...checkpoint.claim,device:'f'.repeat(64)}};
 expect(()=>decodeWalletStorage(JSON.stringify({version:2,exercised:false,installation:altered}))).toThrow();
});
test('boot-bound monotonic time rejects rollback, reboot and excessive skew',()=>{
 const {installationTime}=require('../src/wallet/boardInstallation');
 const base={boot:'12345678-1234-1234-1234-123456789abc',uptimeMs:100000,wallMs:150000};
 expect(installationTime(base,{...base,uptimeMs:102000,wallMs:152000})).toBe(152);
 for(const current of [{...base,boot:'22345678-1234-1234-1234-123456789abc'},
   {...base,uptimeMs:99999},{...base,uptimeMs:102000,wallMs:100000}, {...base,uptimeMs:401000,wallMs:451000}])
  expect(()=>installationTime(base,current)).toThrow();
});
