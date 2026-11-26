// Bundles the kiosk app with the native modules of this directory. Used by scripts/simulate_kiosk_app.py only.
const path = require('node:path');
const projectRoot = path.resolve(__dirname, '../../../../p04-merchant-kiosk');
const base = require(path.join(projectRoot, 'metro.config.js'));

module.exports = {...base, projectRoot, cacheVersion: 'kiosk-simulator', resolver: {...base.resolver, resolveRequest(context, name, platform) {
  const found = /(?:^|\/)specs\/(Native(?:CommerceHttp|Nearby|KioskVault|NusBle))\.ts$/.exec(name);
  return found ? {type: 'sourceFile', filePath: path.join(__dirname, found[1] + '.ts')} : base.resolver.resolveRequest(context, name, platform);
}}};
