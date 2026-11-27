// Test-only entry for the app's native TurboModule and the production provider.
import React,{useEffect,useState} from 'react';
import {AppRegistry,AppState,Text} from 'react-native';
import NativeBrowser from '../../src/specs/NativeDappBrowser.ts';
import {browserProvider} from '../../src/wallet/dapp/provider.ts';
import {browserRpc} from '../../src/wallet/dapp/browserRpc.ts';
import {issueDappGrant} from '../../src/wallet/dapp/gate.ts';

const origin='https://127.0.0.1:18448';
const scope={board:'11'.repeat(32),account:'0x'+'23'.repeat(20),generation:1,chain:11155111,genesis:'0x'+'45'.repeat(32)};
const clock=()=>Math.floor(Date.now()/1000);
async function record(evidence) {
  await fetch(origin+'/evidence',{method:'POST',body:evidence});
}
function NativeWebViewTest() {
  const [result,show]=useState('Testing native WebView');
  useEffect(()=>{
    const first='aa'.repeat(32), next='bb'.repeat(32);
    const grant=id=>issueDappGrant({transport:'browser',session:id,origin,topLevel:true},scope,['eth_accounts','eth_chainId'],clock(),clock()+120);
    let active=grant(first), initialDocument;
    const observed=NativeBrowser?.onRequest(async frame=>{
      const reply=browserRpc(active,scope,frame,clock());
      if (reply.session===first) initialDocument=reply.document;
      await NativeBrowser.respond(reply.session,reply.document,reply.reply);
    });
    const closed=NativeBrowser?.onClosed(async id=>{
      active.revoked=true;
      await record('closed:'+id.slice(0,2));
      if (id===first) {
        active=grant(next);
        await NativeBrowser.open(origin+'/second',next,browserProvider);
        try {
          await NativeBrowser.respond(first,initialDocument,'{"id":1,"result":"bad"}');
          await record('FAIL:old-document-accepted');
        } catch {await record('stale-document-blocked');}
      }
      show('Native WebView session closed');
    });
    const background=AppState.addEventListener('change',state=>{
      if (state==='background' && active.channel.session===next && !active.revoked) {
        active.revoked=true;
        NativeBrowser.close().then(()=>record('background-revoked'));
      }
    });
    const timer=setTimeout(()=>NativeBrowser?.open(origin+'/native',first,browserProvider).catch(error=>record('FAIL:'+error.code)),1600);
    return ()=>{clearTimeout(timer);observed?.remove();closed?.remove();background.remove();NativeBrowser?.close();};
  },[]);
  return <Text accessibilityRole="header">{result}</Text>;
}
AppRegistry.registerComponent('NU54Renter',()=>NativeWebViewTest);
