'use strict';
// Full public-chain identifiers never enter this design fixture from a wallet or RPC.
// Expand only explicit mock tokens; ordinary abbreviated labels remain abbreviated.
function demoValue(value){
 if(typeof value!=='string')return value;
 return value.replace(/\{\{(address|hash):(0x[0-9a-f]{4})…([0-9a-f]{4})\}\}/gi,
  (_,kind,prefix,suffix)=>prefix.toLowerCase().padEnd(kind==='hash'?62:38,'0')+suffix.toLowerCase());
}
