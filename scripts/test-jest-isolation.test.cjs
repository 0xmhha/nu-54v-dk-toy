const assert = require('node:assert/strict');
const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');

const runner = path.join(__dirname, 'test-jest.cjs');

function setup(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'nu54-test-jest-runner-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const project = path.join(root, 'product with spaces');
  const jest = path.join(project, 'node_modules', 'jest');
  const tmp = path.join(root, 'temporary directories');
  fs.mkdirSync(jest, { recursive: true });
  fs.mkdirSync(tmp);
  fs.writeFileSync(path.join(project, 'package.json'), '{}');
  fs.writeFileSync(path.join(jest, 'package.json'), JSON.stringify({ exports: { './bin/jest': './fake.cjs' } }));
  fs.writeFileSync(path.join(jest, 'fake.cjs'), `
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const args = process.argv.slice(2);
const flag = args.indexOf('--cacheDirectory');
const tmp = os.tmpdir();
const cache = flag < 0 ? path.join(tmp, 'jest') : args[flag + 1];
fs.mkdirSync(cache, { recursive: true });
fs.writeFileSync(path.join(cache, 'receipt'), 'caller cache');
console.log(JSON.stringify({ cwd: process.cwd(), args, tmp, cache }));
if (args.includes('--wait')) setInterval(() => {}, 1000);
else if (args.includes('--fail')) process.exitCode = 7;
`);
  return { project, tmp, env: { ...process.env, TMPDIR: tmp, TEMP: tmp, TMP: tmp } };
}

function run(fixture, args) {
  const completed = spawnSync(process.execPath, [runner, ...args], {
    cwd: fixture.project,
    env: fixture.env,
    encoding: 'utf8',
  });
  assert.ok(completed.stdout, completed.stderr);
  return { ...completed, result: JSON.parse(completed.stdout) };
}

test('isolates and removes the default cache on success and failure', t => {
  const fixture = setup(t);
  const args = ['--runInBand', '--testNamePattern', 'a test with spaces'];
  const good = run(fixture, args);
  const bad = run(fixture, ['--fail']);
  assert.equal(good.status, 0);
  assert.equal(bad.status, 7);
  assert.deepEqual(good.result.args, args);
  assert.equal(fs.realpathSync(good.result.cwd), fs.realpathSync(fixture.project));
  assert.notEqual(good.result.tmp, bad.result.tmp);
  assert.ok(good.result.tmp.startsWith(fixture.tmp + path.sep));
  assert.ok(bad.result.tmp.startsWith(fixture.tmp + path.sep));
  assert.deepEqual(fs.readdirSync(fixture.tmp), []);
});

test('leaves a caller-owned cache and preserves literal CLI arguments', t => {
  const fixture = setup(t);
  const callerCache = path.join(fixture.tmp, 'caller cache');
  const args = ['--cacheDirectory', callerCache, '--', '--flag-looking-filter'];
  const { result, status } = run(fixture, args);
  assert.equal(status, 0);
  assert.deepEqual(result.args, args);
  assert.equal(fs.readFileSync(path.join(callerCache, 'receipt'), 'utf8'), 'caller cache');
  assert.deepEqual(fs.readdirSync(fixture.tmp), ['caller cache']);
});

test('cleans an interrupted run while preserving its original termination signal', {
  skip: process.platform === 'win32',
  timeout: 5000,
}, async t => {
  const fixture = setup(t);
  const child = spawn(process.execPath, [runner, '--wait'], {
    cwd: fixture.project,
    env: fixture.env,
    stdio: ['ignore', 'pipe', 'inherit'],
  });
  t.after(() => child.kill('SIGKILL'));
  const result = await new Promise((resolve, reject) => {
    child.stdout.once('data', output => resolve(JSON.parse(output)));
    child.once('error', reject);
    child.once('close', code => reject(new Error(`runner closed before readiness: ${code}`)));
  });
  assert.ok(fs.existsSync(path.join(result.cache, 'receipt')));
  const closed = new Promise(resolve => child.once('close', (status, signal) => resolve({ status, signal })));
  assert.ok(child.kill('SIGTERM'));
  assert.deepEqual(await closed, { status: null, signal: 'SIGTERM' });
  assert.deepEqual(fs.readdirSync(fixture.tmp), []);
});

test('reports a cleanup failure while preserving a Jest failure code', t => {
  const fixture = setup(t);
  const preload = path.join(fixture.project, 'fail-cleanup.cjs');
  fs.writeFileSync(preload, `
const fs = require('node:fs');
const path = require('node:path');
const remove = fs.rmSync;
fs.rmSync = (directory, ...args) => {
  if (path.basename(directory).startsWith('nu54-jest-')) throw new Error('injected removal failure');
  return remove(directory, ...args);
};
`);
  for (const [args, expected] of [[[], 1], [['--fail'], 7]]) {
    const result = spawnSync(process.execPath, ['--require', preload, runner, ...args], {
      cwd: fixture.project,
      env: fixture.env,
      encoding: 'utf8',
    });
    assert.match(result.stderr, /injected removal failure/);
    assert.ok(result.stderr.includes(JSON.parse(result.stdout).tmp));
    assert.equal(result.status, expected);
  }
});
