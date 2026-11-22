require("./query-decoder-interop.cjs");

const {createInterface} = require('node:readline');
const SignClientModule = require('@walletconnect/sign-client');
const SignClient = SignClientModule.SignClient || SignClientModule.default;
const amount = '0x9';
const recipient = '0x' + 'ab'.repeat(20);
const wrongRecipient = '0x' + 'cc'.repeat(20);
let phase = 'initialize';

function announce(type, data = {}) {
  process.stdout.write(JSON.stringify({type, ...data}) + '\n');
}

function memoryStorage() {
  const storage = new Map();
  return {getItem: async key => storage.get(key), setItem: async (key, value) => storage.set(key, value),
    removeItem: async key => storage.delete(key), getKeys: async () => [...storage.keys()], getEntries: async () => [...storage.entries()]};
}

function nextCommand() {
  return new Promise(resolve => {
    const input = createInterface({input: process.stdin});
    input.once('line', line => {input.close(); resolve(line);});
  });
}

async function main() {
  const [relay, mode, contract] = process.argv.slice(2);
  if (!/^wss:\/\/127\.0\.0\.1:[1-9][0-9]{0,4}$/.test(relay) ||
      !['read', 'user-disconnect', 'spend', 'native-spend', 'native-approve', 'manage-first', 'manage-second',
        'native-expiry', 'expiry-fresh', 'native-explore', 'native-refuse-nft', 'native-refuse-batch'].includes(mode))
    throw new Error('Invalid local invocation');
  const refusal = mode.startsWith('native-refuse-');
  const chain = mode === 'native-refuse-batch' ? 'eip155:84532' : 'eip155:11155111';
  const positive = mode === 'native-approve';
  if (positive && !/^0x[a-f0-9]{40}$/.test(contract)) throw new Error('Invalid token fixture');
  const writable = mode === 'spend' || mode === 'native-spend' || mode === 'native-expiry' || positive || refusal;
  const client = await SignClient.init({relayUrl: relay, projectId: 'a'.repeat(32), logger: 'silent',
    storage: memoryStorage(), telemetryEnabled: false, metadata: {name: 'Offline QA DApp',
      description: 'Local reference interoperability', url: writable ? 'https://independent.example' : mode==='manage-first' ? 'https://first.example' :
        mode==='manage-second' ? 'https://second.example' : 'https://reference.example', icons: []}});
  const events = ['accountsChanged', 'chainChanged'];
  const {uri, approval} = await client.connect({
    requiredNamespaces: {eip155: {chains: [chain], methods: writable ? ['eth_accounts','eth_sendTransaction'] : ['eth_accounts','eth_chainId'], events}},
    ...(writable ? {} : {optionalNamespaces: {eip155: {chains: [chain], methods: ['eth_sendTransaction'], events}}}),
  });
  const proposal = client.proposal.getAll()[0];
  // Modern SignClient delegates the proposal subscription to the gateway.
  // Our loopback IRN fixture uses the SDK's explicit relay subscription API.
  await client.core.relayer.subscribe(proposal.pairingTopic);
  announce('pair', {uri, pub: proposal.proposer.publicKey});
  phase = 'approval';
  const session = await approval();
  const request = (method, params) => client.request({topic: session.topic, chainId: chain, request: {method, params}});
  if (mode === 'native-expiry') {
    if ((await request('eth_accounts', [])).length !== 1) throw new Error('Initial account is unavailable');
    announce('initial-read', {topic: session.topic});
    if (await nextCommand() !== 'resume') throw new Error('Invalid resume command');
    phase = 'expiring-transfer';
    const owner = session.namespaces.eip155.accounts[0].split(':')[2];
    const response = request('eth_sendTransaction', [{from: owner, to: recipient, value: amount}]).then(() => true, () => false);
    announce('awaiting-expiry', {topic: session.topic, expiry: session.expiry});
    if (await nextCommand() !== 'check') throw new Error('Invalid expiry command');
    const afterExpiry = request('eth_accounts', []).then(() => true, () => false);
    const answered = reply => Promise.race([reply, new Promise(resolve => setTimeout(() => resolve(false), 3500))]);
    if (await answered(response) || await answered(afterExpiry)) throw new Error('Expired session answered a request');
    announce('expired');
    return;
  } else if (refusal) {
    phase = 'unsupported-intent';
    const owner = session.namespaces.eip155.accounts[0].split(':')[2];
    const word = n => n.toString(16).padStart(64,'0');
    const data = mode === 'native-refuse-nft' ? '0x42842e0e'+owner.slice(2).padStart(64,'0')+
      recipient.slice(2).padStart(64,'0')+word(7) :
      '0xac9650d8'+word(32)+word(1)+word(32)+word(4)+'a9059cbb'.padEnd(64,'0');
    let errorCode;
    try { await request('eth_sendTransaction',[{from:owner,to:recipient,value:'0x0',data}]); }
    catch (error) { errorCode=error?.code; }
    if (errorCode!==5000) throw new Error('Scoped advanced intent did not receive wallet refusal');
    announce('refused',{code:errorCode});
    if (await nextCommand() !== 'continue') throw new Error('Invalid driver command');
  } else if (positive) {
    const owner = session.namespaces.eip155.accounts[0].split(':')[2];
    const approve = amount => request('eth_sendTransaction', [{from:owner,to:contract,value:'0x0',
      data:'0x095ea7b3'+recipient.slice(2).padStart(64,'0')+amount.toString(16).padStart(64,'0')}]);
    for (const [step,limit] of [['over-cap',5000001],['customer-decline',5000000]]) {
      phase = step;
      let reply;
      try { await approve(limit); } catch (error) { reply = error; }
      if (reply?.message==='Request already seen or an approval is pending') phase += '-busy';
      if (reply?.code!==5000 || reply.message!=='Wallet refused this request')
        throw new Error('Expected scoped allowance refusal');
      // The next independent request starts after the wallet finishes the
      // previous relay acknowledgement and releases its pending review.
      await new Promise(resolve => setTimeout(resolve,350));
    }
    phase = 'view-connections';
    let viewed;
    try { await approve(5000000); } catch (error) { viewed=error; }
    if (viewed?.code!==5000 || viewed.message!=='Allowance review ended. Send a new request to continue.')
      throw new Error('Connection view did not retire the request');
    announce('viewed',{topic:session.topic});
    if (await nextCommand()!=='continue') throw new Error('Invalid driver command');
    phase = 'bounded-approval';
    const hash = await approve(5000000).catch(error=>{
      if (error?.message==='Request already seen or an approval is pending') phase += '-busy';
      throw error;
    });
    announce('tx', {hash});
    return;
  } else if (writable) {
    phase = 'wrong-recipient';
    const owner = session.namespaces.eip155.accounts[0].split(':')[2];
    const pay = to => request('eth_sendTransaction', [{from: owner, to, value: amount}]);
    let rejected = false;
    try { await pay(wrongRecipient); } catch { rejected = true; }
    if (!rejected) throw new Error('Customer refusal was not honored');
    phase = 'authorized-payment';
    const hash = await pay(recipient);
    announce('tx', {hash, rejected});
    if (mode === 'native-spend') {
      if (await nextCommand() !== 'continue') throw new Error('Invalid driver command');
      const accounts = await request('eth_accounts', []);
      if (accounts.length !== 1 || accounts[0] !== owner) throw new Error('Resumed account mismatch');
      announce('resumed', {sameTopic: true});
    }
  } else {
    const value = await request('eth_chainId', []);
    const accounts = await request('eth_accounts', []);
    const methods = session.namespaces.eip155.methods;
    if (value !== '0xaa36a7' || !Array.isArray(accounts) || accounts.length !== 1 ||
        methods.length !== 2 || !methods.includes('eth_accounts') || !methods.includes('eth_chainId'))
      throw new Error('Read-only scope mismatch');
    let denied = false;
    try {
      await request('eth_sendTransaction', [{from: accounts[0], to: recipient, value: amount}]);
    } catch { denied = true; }
    if (!denied) throw new Error('Unverified spend was not denied');
    announce('read', {chain: value, accountCount: accounts.length, denied, ...(mode==='expiry-fresh' ? {topic: session.topic} : {})});
    if (mode==='native-explore') return;
    if (mode==='expiry-fresh') {
      if (await nextCommand() !== 'resume') throw new Error('Invalid fresh-session resume command');
      const resumed = await request('eth_accounts', []);
      if (resumed.length !== 1 || resumed[0] !== accounts[0]) throw new Error('Fresh authorization did not survive restart');
      announce('fresh-resumed');
      return;
    }
    if (mode==='manage-second') {
      if (await nextCommand()!=='check') throw new Error('Invalid management command');
      const resumed=await request('eth_accounts',[]);
      if (resumed.length!==1 || resumed[0]!==accounts[0] || client.session.getAll().length!==1)
        throw new Error('Unselected session did not remain authorized');
      announce('unselected-active');
    }
    if (await nextCommand() !== 'continue') throw new Error('Invalid driver command');
    if (mode === 'user-disconnect' || mode.startsWith('manage-')) {
      phase = 'wallet-initiated-session-close';
      for (let elapsed = 0; elapsed < 320 && client.session.getAll().length; elapsed++) {
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      const activeSessions = client.session.getAll().length;
      if (activeSessions !== 0) throw new Error('Wallet session remained active');
      phase = 'request-after-disconnect';
      let deniedAfterEnd = false;
      try { await request('eth_accounts', []); } catch { deniedAfterEnd = true; }
      if (!deniedAfterEnd) throw new Error('Ended session answered a new request');
      announce('disconnected', {activeSessions, deniedAfterEnd});
      return;
    }
    if (await request('eth_chainId', []) !== value) throw new Error('Resumed chain mismatch');
    announce('resumed', {sameTopic: true});
  }
  await client.disconnect({topic: session.topic, reason: {code: 6000, message: 'User disconnected'}});
  announce('closed');
}

// Keep the pending SDK approval alive while the native user completes review.
// A Promise alone does not keep Node running if the SDK idles its relay socket.
const keepAlive = setInterval(() => {}, 1000);
main().catch(error => {
  // Avoid writing proposal URIs, session identifiers or SDK traces to evidence.
  announce('failed', {phase, code: Number.isInteger(error?.code) ? error.code : undefined});
  process.exit(1);
}).then(() => {
  clearInterval(keepAlive);
  process.exit(0);
});
