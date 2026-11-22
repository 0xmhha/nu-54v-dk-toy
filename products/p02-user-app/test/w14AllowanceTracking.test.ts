import {queryEvmAllowanceChange,type EvmRpc} from '../src/wallet/evmTransfer';

test('only reports the canonical allowance after the original approval is finalized',async()=>{
  const genesis='0x'+'ab'.repeat(32), hash='0x'+'12'.repeat(32);
  const includedBlock='0x'+'23'.repeat(32), finalizedBlock='0x'+'34'.repeat(32);
  const owner='0x'+'11'.repeat(20), token='0x'+'22'.repeat(20), spender='0x'+'33'.repeat(20);
  const original={transactionHash:hash,blockHash:includedBlock,blockNumber:'0x6',status:'0x1'};
  let receipt:typeof original|null=null;
  let finalized=false, reorgDuringRead=false, advanceDuringRead=false, advanced=false, allowanceReads=0;
  const rpc:EvmRpc=async(method,args)=>{
    switch(method) {
      case 'eth_chainId': return '0xaa36a7';
      case 'eth_getTransactionReceipt':
        expect(args).toEqual([hash]);
        return receipt;
      case 'eth_getBlockByNumber':
        if (args[0]==='0x0') return {hash:genesis};
        if (args[0]==='0x6') return {hash:includedBlock};
        if (args[0]==='finalized' && advanced) return {number:'0x8',hash:'0x'+'45'.repeat(32)};
        return {number:finalized ? '0x7' : '0x5',hash:finalizedBlock};
      case 'eth_call':
        allowanceReads++;
        expect(args).toEqual([{to:token,data:'0xdd62ed3e'+owner.slice(2).padStart(64,'0')+spender.slice(2).padStart(64,'0')},
          {blockHash:finalizedBlock,requireCanonical:true}]);
        if (reorgDuringRead) receipt=null;
        if (advanceDuringRead) advanced=true;
        return '0x'+(4000000).toString(16).padStart(64,'0');
      default: throw new Error(method);
    }
  };
  const query=()=>queryEvmAllowanceChange(rpc,'11155111',genesis,hash,token,owner,spender);
  expect(await query()).toEqual({state:'pending'});
  receipt=original;
  expect(await query()).toEqual({state:'included'});
  finalized=true;
  receipt={...original,status:'0x0'};
  expect(await query()).toEqual({state:'reverted'});
  expect(allowanceReads).toBe(0);
  receipt=original;
  expect(await query()).toEqual({state:'finalized',value:4000000n});
  reorgDuringRead=true;
  expect(await query()).toEqual({state:'pending'});
  reorgDuringRead=false;
  receipt=original;
  advanceDuringRead=true;
  expect(await query()).toEqual({state:'pending'});
  expect(allowanceReads).toBe(3);
});
