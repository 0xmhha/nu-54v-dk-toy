// Serves the store of ../app/world.ts to the kiosk app on an emulator: its registration, its server connection
// and its journals. scripts/simulate_kiosk_app.py starts it; nothing here is a check.
import * as http from 'node:http';
import {randomBytes} from 'node:crypto';
import {writeSync} from 'node:fs';
import * as world from '../app/world';

const port = 8790, life = Number(process.env.NU54_ORDER_FLOW_MINUTES) * 60_000;
const journals = new Map<string, string>();
// The kiosk's registration as an installer would have written it, for the store's terminal certificate.
const profile = JSON.stringify({
  scope: {role: 'terminal', tenantRef: world.fixture.tenant, branchRef: world.fixture.branch, subjectRef: world.fixture.terminal.subject,
    expiresAt: world.fixture.expiresAt},
  pin: 'a'.repeat(64), payment: world.fixture.payment,
});

const modules: Record<string, Record<string, (...args: never[]) => unknown>> = {
  CommerceHttp: {
    loadProfile: (slot: string) => slot === 'terminal' ? profile : '',
    publicIdentity: () => 'key',
    configure: () => undefined,
    randomBytes: (size: number) => randomBytes(size).toString('base64'),
    loadJournal: (name: string) => journals.get(name) ?? '',
    saveJournal: (name: string, value: string) => { journals.set(name, value); },
    exchange: (_slot: string, body: string) => world.exchange(world.fixture.terminal, body),
    exchangeScoped: (_slot: string, _digest: string, body: string) => world.exchange(world.fixture.terminal, body),
  },
  // No phone comes near this kiosk; the radio stays quiet.
  Nearby: {publicIdentity: () => 'key', paymentPublicIdentity: () => 'key', configure: () => undefined, configurePayment: () => undefined,
    start: () => undefined, stop: () => undefined, stopPayment: () => undefined, scan: () => '[]', scanPayment: () => '[]'},
  KioskVault: {getSetting: () => null, putSetting: () => undefined, takeFile: () => null, unwrapKey: () => null,
    randomBytes: (n: number) => randomBytes(n).toString('base64')},
  NusBle: {},
};

let finish = () => {};
test('serve the kiosk until it is stopped', async () => {
  const server = http.createServer((request, reply) => {
    const send = (value: unknown) => { reply.setHeader('Content-Type', 'application/json'); reply.end(JSON.stringify(value)); };
    if (request.headers.origin) { reply.statusCode = 403; reply.end(); return; }
    const url = new URL(request.url!, 'http://host');
    // The kiosk listens for nearby phones; none come.
    if (url.pathname === '/events') { setTimeout(() => send({next: 0, events: []}), 10_000); return; }
    let raw = '';
    request.setEncoding('utf8').on('data', text => { raw += text; }).on('end', async () => {
      try {
        const body = JSON.parse(raw || '{}');
        if (url.pathname === '/step') {
          if (body.step === 'fixture') return send({value: {endpoint: world.fixture.endpoint, branch: world.fixture.branch,
            cert: world.fixture.kitchen.cert, key: world.fixture.kitchen.key}});
          if (body.step === 'stop') { finish(); return send({value: null}); }
          throw new Error('unknown step ' + body.step);
        }
        const found = Object.hasOwn(modules, body.module) ? modules[body.module] : {};
        const run = Object.hasOwn(found, body.method) ? found[body.method] : undefined;
        if (typeof run !== 'function') throw Object.assign(new Error(`not simulated: ${body.module}.${body.method}`), {code: 'NOT_SIMULATED'});
        const value = await (run as (...args: unknown[]) => unknown)(...body.args) ?? null;
        if (body.method.startsWith('exchange')) {
          const {command} = JSON.parse(body.args.at(-1));
          const answer = String(value);
          writeSync(1, `KIOSK ${command?.operation} ${answer.includes('"status":"error"') ? answer.slice(0, 300) : 'ok'}\n`);
        }
        send({value});
      } catch (error) {
        writeSync(1, `KIOSK ${body.module}.${body.method} failed: ${(error as Error).message}\n`);
        send({error: (error as Error).message || 'error', code: (error as {code?: string}).code});
      }
    });
  });
  await new Promise<void>((resolve, reject) => { server.once('error', reject).listen(port, '127.0.0.1', resolve); });
  writeSync(1, 'SIMULATOR-READY\n');
  await new Promise<void>(resolve => { finish = resolve; setTimeout(resolve, life - 120_000); });
  world.stopWorld();
  server.closeAllConnections();
  server.close();
}, 60 * 60_000);
