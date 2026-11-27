import {beginPractice, practiceDecision, readPractice} from '../src/wallet/practiceFlow';

test('practice requires an explicit correct decision and resumes without a payment', () => {
  const matching = beginPractice('matching');
  expect(practiceDecision(matching, 'approve').state).toBe('done');
  const wrong = beginPractice('wrongAmount');
  expect(practiceDecision(wrong, 'approve').state).toBe('practice');
  expect(practiceDecision(wrong, 'reject').state).toBe('reject');
  expect(readPractice(JSON.stringify(wrong))).toEqual(wrong);
  expect(readPractice('{"scenario":"matching","state":"sign"}')).toBeUndefined();
  expect(readPractice('{"scenario":"wrongAmount","state":"done"}')).toBeUndefined();
});

test('supported pre-practice wallet profile remains readable', () => {
  const {decodeWalletStorage} = require('../src/wallet/profile');
  expect(decodeWalletStorage('{"version":2,"exercised":false}')).toEqual({version:2,exercised:false});
});
