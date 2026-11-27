import React from 'react';
import Renderer from 'react-test-renderer';
import {Text} from 'react-native';
import type {CommerceReadyBoard} from '@nu54/protocol';
import {ReadyBand, ReadyBoardView, useReadyBoard, boardPollMs} from '../src/kiosk/ReadyBoard';

const board = (ready: number[], making: number[] = [13], received: number[] = [15]): CommerceReadyBoard =>
  ({checkedAt: '2026-11-26T01:17:05Z', ready, making, received});
const text = (tree: Renderer.ReactTestRenderer) => JSON.stringify(tree.toJSON());

test('the whole board shows four ready numbers large, the rest in a line, and what is being made and received', async () => {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<ReadyBoardView board={board([21, 20, 19, 18, 17, 16])} lost={false} fresh={[21]} ko />);
  });
  for (const n of [21, 20, 19, 18]) expect(tree.root.findByProps({accessibilityLabel: `준비된 번호 ${n}`})).toBeTruthy();
  expect(text(tree)).toContain('그 밖에 준비된 번호 17 · 16');
  expect(text(tree)).toContain('새로 준비된 번호 21'); // read out once
  expect(text(tree)).toContain('만드는 중');
  expect(text(tree)).toContain('접수');
  expect(text(tree)).not.toContain('자동 주방');
  await Renderer.act(async () => tree.update(<ReadyBoardView board={{...board([21]), autoKitchen: true}} lost={false} fresh={[]} ko />));
  expect(text(tree)).toContain('시험 매장 · 자동 주방으로 운영 중');
  await Renderer.act(async () => tree.update(<ReadyBoardView board={board([21])} lost fresh={[]} ko />));
  expect(text(tree)).toContain('연결이 끊겼습니다 · 마지막 확인');
});

test('the band shows the four newest ready numbers and nothing when none are ready', async () => {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<ReadyBand board={board([9, 8, 7, 6, 5])} ko />); });
  expect(tree.root.findByProps({accessibilityLabel: '준비된 번호 9, 8, 7, 6'})).toBeTruthy();
  await Renderer.act(async () => tree.update(<ReadyBand board={board([])} ko />));
  expect(tree.toJSON()).toBeNull();
});

test('only numbers that became ready after the board came up are marked new, and a failed read keeps the last board', async () => {
  jest.useFakeTimers();
  const reads: (CommerceReadyBoard | Error)[] = [board([11]), board([12, 11]), new Error('offline')];
  const read = jest.fn(async () => { const next = reads.shift()!; if (next instanceof Error) throw next; return next; });
  let seen: ReturnType<typeof useReadyBoard> | undefined;
  function Probe() { seen = useReadyBoard(read, true); return <Text>{JSON.stringify(seen.fresh)}</Text>; }
  await Renderer.act(async () => { Renderer.create(<Probe />); });
  expect(seen!.board?.ready).toEqual([11]);
  expect(seen!.fresh).toEqual([]);
  await Renderer.act(async () => { jest.advanceTimersByTime(boardPollMs); });
  expect(seen!.fresh).toEqual([12]);
  await Renderer.act(async () => { jest.advanceTimersByTime(boardPollMs); });
  expect(seen!.lost).toBe(true);
  expect(seen!.board?.ready).toEqual([12, 11]);
  jest.useRealTimers();
});
