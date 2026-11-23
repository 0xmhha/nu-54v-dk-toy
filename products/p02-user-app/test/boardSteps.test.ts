import {readFileSync} from 'fs';
import {join} from 'path';
import {boardFlows, boardPosition, boardSteps, boardStepsAhead} from '../src/wallet/boardSteps';

// The design book is the reviewed source for board steps; the app must show the same words.
const book = JSON.parse(readFileSync(join(__dirname, '../../../docs/content/commercial/screens/board.json'), 'utf8'));

test('board steps match the design book', () => {
  expect(Object.keys(boardSteps).sort()).toEqual(Object.keys(book.steps).sort());
  for (const [id, step] of Object.entries(book.steps) as [string, Record<string, unknown>][]) {
    const mine = boardSteps[id as keyof typeof boardSteps];
    expect({label: mine.label, do: mine.do, buttons: mine.buttons, led: mine.led, app: mine.app, pending: mine.pending})
      .toEqual({label: step.label, do: step.do, buttons: step.buttons, led: step.led, app: step.app, pending: step.pending});
  }
  // The order flow of the spending profile is not in the book yet; it only reuses the book's steps.
  const {order, ...reviewed} = boardFlows;
  expect(reviewed).toEqual(book.flows);
  expect(order.steps).toEqual(['approve', 'pin', 'done']);
});

test('approval copy never makes the phone the reference', () => {
  for (const step of Object.values(boardSteps)) {
    for (const sentence of [step.do, step.app]) {
      if (/(맞으면|같으면|일치하면)[^.]*(SW1|승인)/.test(sentence)) expect(sentence).toContain('독립');
    }
  }
});

test('position and next buttons follow the flow', () => {
  expect(boardPosition({flow: 'sign', step: 'display'})).toEqual({index: 0, total: 4});
  expect(boardStepsAhead({flow: 'sign', step: 'display'})).toEqual(['approve', 'pin']);
  expect(boardStepsAhead({flow: 'register', step: 'pin'})).toEqual([]);
  expect(boardPosition({flow: 'sign', step: 'failed'})).toBeUndefined();
  expect(boardStepsAhead({flow: 'sign', step: 'failed'})).toEqual([]);
});
