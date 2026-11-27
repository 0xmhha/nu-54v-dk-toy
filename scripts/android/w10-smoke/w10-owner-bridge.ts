import OwnerWallet from '../../../products/p02-user-app/src/specs/NativeOwnerWallet.ts';
import nearby from '../../ios/scene-smoke/fixtures/k10-nearby-adapter.ts';

type Setup = {control:string; token:string; hostBoard?:boolean};
let setup: Setup | undefined;

async function board(route: string, body?: string) {
  if (!setup?.hostBoard) throw Error('LAB_OWNER_NOT_REGISTERED');
  const reply = await fetch(setup.control + '/owner/' + route, {
    method:'POST', headers:{Authorization:'Bearer '+setup.token}, body,
  });
  if (!reply.ok) throw Error('LAB_OWNER_BRIDGE');
  return reply.json();
}

export const nearbyWithBoard = {
  ...nearby,
  async configure(raw:string) {
    await nearby.configure(raw);
    const candidate = JSON.parse(raw) as Setup;
    if (!/^http:\/\/127\.0\.0\.1:[0-9]+$/.test(candidate.control) || !candidate.hostBoard)
      throw Error('LAB_OWNER_LOOPBACK_REQUIRED');
    setup = candidate;
    if (!OwnerWallet) throw Error('LAB_OWNER_WALLET_REQUIRED');
    await board('stage','profile-read');
    if (!(await OwnerWallet.loadProfile())) {
      await board('stage','native-identity');
      const identity = await OwnerWallet.publicIdentity();
      await board('stage','host-enrollment');
      const profile = await board('enroll',identity);
      await board('stage','profile-save');
      await OwnerWallet.saveProfile(JSON.stringify(profile));
    }
    await board('stage','owner-ready');
  },
};

const scans = new Set<(event:string)=>void>();
const fragments = new Set<(event:string)=>void>();
const disconnections = new Set<()=>void>();
let epoch = 0;

function listen<T>(listeners:Set<T>, listener:T) {
  listeners.add(listener);
  return {remove() {listeners.delete(listener);}};
}

async function acceptFragments(active:number) {
  for (;;) {
    if (active !== epoch) return;
    try {
      const message = await board('read');
      if (active !== epoch) return;
      if (typeof message.fragment === 'string') {
        for (const listener of fragments) listener(message.fragment);
      } else if (message.closed) break;
    } catch { break; }
  }
  if (active === epoch) for (const listener of disconnections) listener();
}

export const ownerBle = {
  onScan: (receive:(event:string)=>void) => listen(scans,receive),
  onFragment: (receive:(event:string)=>void) => listen(fragments,receive),
  onDisconnect: (receive:()=>void) => listen(disconnections,receive),
  async scanOwner() {
    for (const receive of scans) receive(JSON.stringify({address:'host-board',name:'검증 보드'}));
  },
  async connectOwner(handle:string) {
    if (handle !== 'host-board') throw Error('LAB_OWNER_DEVICE');
    await board('open');
    void acceptFragments(++epoch);
    return 247;
  },
  async writeFragment(body:string) { await board('write',body); },
  async disconnect() { ++epoch; await board('close'); },
};
