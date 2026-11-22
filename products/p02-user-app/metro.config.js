const path = require('path');
const {approvedOrigin} = require('./build-tools/public-travel-origin');
const {getDefaultConfig, mergeConfig} = require('@react-native/metro-config');

// Monorepo: let Metro see the workspace root so @nu54/protocol resolves ([N25]).
const workspaceRoot = path.resolve(__dirname, '../..');

const config = {
  cacheVersion: `p09:${approvedOrigin(process.env.NU54_P09_PUBLIC_ORIGIN)}`,
  watchFolders: [workspaceRoot],
  resolver: {
    // Workspace packages can have different peer-resolution trees. React hooks
    // and the renderer must always resolve from the app's single installation.
    resolveRequest(context, moduleName, platform) {
      if (moduleName === 'react' || moduleName.startsWith('react/') ||
          moduleName === 'react-native' || moduleName.startsWith('react-native/')) {
        const appModule = moduleName.startsWith('react-native/')
          ? path.join(path.dirname(require.resolve('react-native/package.json', {paths: [__dirname]})), moduleName.slice('react-native/'.length))
          : require.resolve(moduleName, {paths: [__dirname]});
        return context.resolveRequest(context, appModule, platform);
      }
      return context.resolveRequest(context, moduleName, platform);
    },
    nodeModulesPaths: [
      path.resolve(__dirname, 'node_modules'),
      path.resolve(workspaceRoot, 'node_modules'),
    ],
  },
};

module.exports = mergeConfig(getDefaultConfig(__dirname), config);
