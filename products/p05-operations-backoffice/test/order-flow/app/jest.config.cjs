// The phone and kiosk side of scripts/test_order_flow.py. It runs the app sources with the P02 test setup.
const path = require('node:path');
const p02 = path.resolve(__dirname, '../../../../p02-user-app');

module.exports = {
  ...require(path.join(p02, 'jest.config.js')),
  rootDir: p02,
  roots: [__dirname],
  testMatch: ['**/orderFlow.test.tsx'],
  modulePaths: [path.join(p02, 'node_modules')],
  testTimeout: 180_000,
  verbose: false,
};

// scripts/test_order_flow.py --only
if (process.env.NU54_ORDER_FLOW_ONLY) module.exports.testNamePattern = process.env.NU54_ORDER_FLOW_ONLY;
