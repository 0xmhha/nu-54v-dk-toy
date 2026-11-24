// Stands in for a native module of the user app in scripts/simulate_user_app.py; host.test.ts answers.
// Only metro.config.js in this directory can put it into a bundle.
const HOST = 'http://127.0.0.1:8790';
type Listener = (value: string) => void;
const listeners = new Map<string, Set<Listener>>();
let polling = false, since = -1;
async function poll() {
  if (polling) return;
  polling = true;
  for (;;) {
    try {
      const reply = await fetch(`${HOST}/events?since=${since}`);
      const {next, events} = await reply.json() as {next: number; events: {name: string; value: string}[]};
      since = next;
      for (const event of events) for (const run of [...(listeners.get(event.name) ?? [])]) run(event.value);
    } catch { await new Promise<void>(resolve => setTimeout(resolve, 500)); }
  }
}
export function nativeModule(name: string): any {
  void poll();
  const made = new Map<string, unknown>();
  return new Proxy({}, {get(_target, key) {
    if (typeof key !== 'string' || key === 'then' || key === 'toJSON' || key === '$$typeof') return undefined;
    if (made.has(key)) return made.get(key);
    const value = /^on[A-Z]/.test(key)
      ? (run: Listener) => {
        const event = `${name}.${key}`;
        if (!listeners.has(event)) listeners.set(event, new Set());
        listeners.get(event)!.add(run);
        return {remove: () => { listeners.get(event)!.delete(run); }};
      }
      : async (...args: unknown[]) => {
        const reply = await fetch(`${HOST}/call`, {method: 'POST', body: JSON.stringify({module: name, method: key, args})});
        const out = await reply.json() as {value?: unknown; error?: string; code?: string};
        if (out.error) throw Object.assign(new Error(out.error), {code: out.code});
        // A scan ends only after its results arrived over the other connection.
        if (/^scan/.test(key)) await new Promise<void>(resolve => setTimeout(resolve, 900));
        return out.value;
      };
    made.set(key, value);
    return value;
  }});
}

// uiautomator reads the screen only when it is quiet, so one-second countdowns tick every three seconds here.
const world = globalThis as any;
if (!world.slowTicks) {
  world.slowTicks = true;
  const real = world.setInterval;
  world.setInterval = (run: unknown, delay: number, ...rest: unknown[]) => real(run, delay >= 250 && delay <= 1000 ? 3000 : delay, ...rest);
}
