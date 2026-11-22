import {verifyDeploymentPolicy, type Profile} from '../src/market/lab.ts';

const target = (digit:string) => ({address:'0x'+digit.repeat(40),codeHash:'0x'+digit.repeat(64)});
const profile:Profile = {environment:'p08-isolated-8283-v1',chainId:8283,genesis:'0x'+'1'.repeat(64),
  deploymentHash:'0x'+'2'.repeat(64),deploymentHeight:42,quoteSigner:target('7').address,
  venue:target('3'),base:target('4'),quote:target('5')};

test('requires the installed policy and binds every isolated deployment component',()=>{
  const digest=verifyDeploymentPolicy('loopback',profile);
  expect(verifyDeploymentPolicy(digest,profile)).toBe(digest);
  expect(()=>verifyDeploymentPolicy('',profile)).toThrow();
  for (const changed of [
    {...profile,chainId:1}, {...profile,genesis:target('8').codeHash},
    {...profile,deploymentHash:target('8').codeHash}, {...profile,deploymentHeight:43},
    {...profile,quoteSigner:target('8').address}, {...profile,venue:target('8')},
    {...profile,base:{...profile.base,address:target('8').address}},
    {...profile,quote:{...profile.quote,codeHash:target('9').codeHash}},
  ]) expect(()=>verifyDeploymentPolicy(digest,changed)).toThrow();
});
