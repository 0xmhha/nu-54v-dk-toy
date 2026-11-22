// Lab-only scanner and link carrier for the simulated owner board.
const scans=new Set();
const empty=()=>({remove(){}});
export default {
  loadDevices:async()=> '[]',
  hasPermissions:async()=> true,
  scanOwner:async()=>{
    for(const listener of scans) listener(JSON.stringify({address:'qa-board',name:'Loopback P01 demo board'}));
  },
  onScan:listener=>{scans.add(listener);return {remove:()=>scans.delete(listener)};},
  connectOwner:async handle=>{if(handle!=='qa-board') throw new Error('Unexpected lab board');return 247;},
  onFragment:empty,
  onDisconnect:empty,
  writeFragment:async()=>{},
  disconnect:async()=>{},
};
