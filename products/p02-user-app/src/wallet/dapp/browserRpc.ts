import {authorizeDappRequest,type DappChannel,type DappGrant,type DappScope} from './gate.ts';

export function parseBrowserRpc(frame:string) {
  const outer=JSON.parse(frame);
  if (!outer || typeof outer.session!=='string' || typeof outer.origin!=='string' ||
      typeof outer.document!=='string' || !/^[0-9a-f-]{36}$/i.test(outer.document) ||
      typeof outer.request!=='string' || outer.request.length>4096) throw new Error('DAPP_FRAME');
  const request=JSON.parse(outer.request);
  if (!request || !Number.isSafeInteger(request.id) || request.id<1 || typeof request.method!=='string')
    throw new Error('DAPP_REQUEST');
  const channel:DappChannel={transport:'browser',session:outer.session,origin:outer.origin,topLevel:true};
  return {channel,request,document:outer.document as string};
}

export function browserRpc(grant:DappGrant,scope:DappScope,frame:string,now:number) {
  const {channel,request,document}=parseBrowserRpc(frame);
  let response;
  try {
    const allowed=authorizeDappRequest(grant,channel,scope,
      {method:request.method==='eth_requestAccounts' ? 'eth_accounts' : request.method,params:request.params},now);
    if (allowed.kind==='accounts') response={result:allowed.accounts};
    else if (allowed.kind==='chain') response={result:allowed.chain};
    else response={error:{code:4200,message:'This signing method is unavailable in this browser session'}};
  } catch {
    response={error:{code:4100,message:'Request outside the approved wallet scope'}};
  }
  return {session:channel.session,document,reply:JSON.stringify({id:request.id,...response})};
}
