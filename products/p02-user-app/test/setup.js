/* eslint-env jest */
// Screens are tested as someone who already agreed; consent.test.tsx tests the question itself.
jest.mock('../src/legal/ConsentGate.tsx', () => ({ConsentGate: ({children}) => children}));
