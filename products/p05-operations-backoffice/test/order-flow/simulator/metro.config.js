// Bundles the user app with the native modules of this directory. Used by scripts/simulate_user_app.py only.
const path = require('node:path');
const projectRoot = path.resolve(__dirname, '../../../../p02-user-app');
const base = require(path.join(projectRoot, 'metro.config.js'));

// A run with a customer entrance (--route entrance) names the module its host wrote: the entrance and the order
// contract of that run. Only this configuration puts it in the place of customerEntranceDeployment.ts, so the app's
// own bundler and its tests keep the repository's file, which binds nothing. binding.test.ts holds both.
const binding = process.env.NU54_SIMULATOR_ORDER_BINDING ? path.resolve(process.env.NU54_SIMULATOR_ORDER_BINDING) : undefined;

module.exports = {...base, projectRoot, cacheVersion: 'user-app-simulator',
  watchFolders: binding ? [...base.watchFolders, path.dirname(binding)] : base.watchFolders,
  resolver: {...base.resolver, resolveRequest(context, name, platform) {
    if (binding && /(?:^|\/)customerEntranceDeployment\.ts$/.test(name)) return {type: 'sourceFile', filePath: binding};
    const found = /(?:^|\/)specs\/(Native(?:CommerceHttp|Nearby|OwnerWallet|RenterBle|ShopQr))\.ts$/.exec(name);
    return found ? {type: 'sourceFile', filePath: path.join(__dirname, found[1] + '.ts')} : base.resolver.resolveRequest(context, name, platform);
  }}};
