// Bundles the user app with the native modules of this directory. Used by scripts/simulate_user_app.py only.
const path = require('node:path');
const projectRoot = path.resolve(__dirname, '../../../../p02-user-app');
const base = require(path.join(projectRoot, 'metro.config.js'));

module.exports = {...base, projectRoot, cacheVersion: 'user-app-simulator', resolver: {...base.resolver, resolveRequest(context, name, platform) {
  const found = /(?:^|\/)specs\/(Native(?:CommerceHttp|Nearby|OwnerWallet|RenterBle|ShopQr))\.ts$/.exec(name);
  return found ? {type: 'sourceFile', filePath: path.join(__dirname, found[1] + '.ts')} : base.resolver.resolveRequest(context, name, platform);
}}};
