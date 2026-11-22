const { spawn } = require('node:child_process');
const { mkdtempSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');

const cwd = process.cwd();
const jest = require.resolve('jest/bin/jest', { paths: [cwd] });
const temporary = mkdtempSync(join(resolve(tmpdir()), 'nu54-jest-'));

function clean() {
  try {
    rmSync(temporary, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 });
    return true;
  } catch (error) {
    console.error(`Could not remove Jest temporary directory ${temporary}: ${error.message}`);
    return false;
  }
}
process.once('exit', clean);

// Jest derives its default cache from os.tmpdir(). A cacheDirectory passed
// through the CLI or the project's Jest config still takes precedence.
const env = { ...process.env, TMPDIR: temporary };
if (process.platform === 'win32') Object.assign(env, { TEMP: temporary, TMP: temporary });

const child = spawn(process.execPath, [jest, ...process.argv.slice(2)], {
  cwd,
  env,
  stdio: 'inherit',
});
const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
let forwarded;
const handlers = signals.map(signal => {
  const handler = () => {
    forwarded ??= signal;
    child.kill(signal);
  };
  process.on(signal, handler);
  return handler;
});

child.once('error', error => console.error(error.message));
child.once('close', (status, signal) => {
  const cleaned = clean();
  process.removeListener('exit', clean);
  signals.forEach((name, index) => process.removeListener(name, handlers[index]));
  if (forwarded || signal) process.kill(process.pid, forwarded || signal);
  else process.exitCode = !cleaned && status === 0 ? 1 : (status ?? 1);
});
