// The checks of the stand-ins themselves; host.test.ts beside them is the host of a simulation, not a check.
//   cd products/p02-user-app && node ../../scripts/test-jest.cjs --config ../p05-operations-backoffice/test/order-flow/simulator/checks.jest.config.cjs
module.exports = {...require('../app/jest.config.cjs'), roots: [__dirname], testMatch: ['**/binding.test.ts', '**/entranceHttp.test.ts'], testNamePattern: undefined,
  testTimeout: 60_000};
