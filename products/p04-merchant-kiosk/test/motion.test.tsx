import React from 'react';
import Renderer from 'react-test-renderer';
import {AccessibilityInfo, Text} from 'react-native';
import {MaskRise, PaidMark, PaymentSteps, Pop, SlideIn} from '../src/kiosk/Motion';

const render = async (element: React.ReactElement) => {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(element); });
  return tree;
};
const rings = (tree: Renderer.ReactTestRenderer) =>
  tree.root.findAll(node => node.props.pointerEvents === 'none' && typeof node.type === 'string').length;

afterEach(() => jest.restoreAllMocks());

test('with motion allowed, the paid mark spreads one ring; with less motion asked for, it stands still', async () => {
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockResolvedValue(false);
  const moving = await render(<PaidMark label="결제 완료" />);
  expect(rings(moving)).toBe(1);
  expect(moving.root.findByProps({accessibilityLabel: '결제 완료'})).toBeTruthy();
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockResolvedValue(true);
  const still = await render(<PaidMark label="결제 완료" />);
  expect(rings(still)).toBe(0);
});

test('the number, the new line and the count stay readable whatever moves', async () => {
  jest.spyOn(AccessibilityInfo, 'isReduceMotionEnabled').mockResolvedValue(false);
  const tree = await render(<>
    <MaskRise height={150}><Text>17</Text></MaskRise>
    <SlideIn><Text>치즈케이크 ×1</Text></SlideIn>
    <Pop value={2}><Text>2</Text></Pop>
  </>);
  const text = JSON.stringify(tree.toJSON());
  for (const part of ['17', '치즈케이크 ×1', '2']) expect(text).toContain(part);
  await Renderer.act(async () => tree.update(<Pop value={3}><Text>3</Text></Pop>));
  expect(JSON.stringify(tree.toJSON())).toContain('3');
});

test('payment steps name the step now and the finished ones', async () => {
  const tree = await render(<PaymentSteps label="결제 단계" steps={['주문 확인', '휴대전화 승인', '결제 확인', '완료']} current={1} />);
  expect(tree.root.findByProps({accessibilityLabel: '1 주문 확인 끝'})).toBeTruthy();
  expect(tree.root.findByProps({accessibilityLabel: '2 휴대전화 승인, 지금'})).toBeTruthy();
  expect(tree.root.findByProps({accessibilityLabel: '4 완료'})).toBeTruthy();
});
