import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {Linking} from 'react-native';
import App from '../src/App';

const mockNativeOrigin = jest.fn(async () => 'https://ops.training.example');
const mockInstalledPolicy = jest.fn(async () => '0x' + 'a'.repeat(64));

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/specs/NativeMarketLab.ts', () => ({
  __esModule: true,
  default: {
    deploymentPolicy: () => mockInstalledPolicy(),
    operatorOrigin: () => mockNativeOrigin(),
  },
}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {loadJournal: async () => ''},
}));

afterEach(() => {
  jest.restoreAllMocks();
  mockInstalledPolicy.mockResolvedValue('0x' + 'a'.repeat(64));
  mockNativeOrigin.mockResolvedValue('https://ops.training.example');
});

test('market operations opens only the installed origin and the exact return link opens learning home', async () => {
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue('nu54://market-learning?deployment=fixture');
  let receive!: (event: {url: string}) => void;
  jest.spyOn(Linking, 'addEventListener').mockImplementation((_, listener) => {
    receive = listener;
    return {remove() {}} as ReturnType<typeof Linking.addEventListener>;
  });
  const open = jest.spyOn(Linking, 'openURL').mockResolvedValue(undefined);
  let view!: ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {
    view = ReactTestRenderer.create(<App />);
  });
  const buttons = (label: string) => view.root.findAll(node =>
    node.props.accessibilityLabel === label && typeof node.props.onPress === 'function');
  expect(buttons('시장 운영 모니터')).toHaveLength(0);
  await ReactTestRenderer.act(async () => receive({url: 'nu54://market-learning'}));
  expect(buttons('시장 운영 모니터')).toHaveLength(1);
  await ReactTestRenderer.act(async () => buttons('시장 운영 모니터')[0].props.onPress());
  expect(open).toHaveBeenCalledWith('https://ops.training.example');
  await ReactTestRenderer.act(async () => view.unmount());
});

test.each([
  ['loopback', 'https://ops.training.example'],
  ['0x' + 'a'.repeat(64), 'https://ops.training.example/path'],
  ['0x' + 'a'.repeat(64), 'https://ops.training.example:65536'],
  ['0x' + 'a'.repeat(64), 'https://ops.training.example?deployment=fixture'],
  ['0x' + 'a'.repeat(64), 'https://ops.training.example@another.example'],
])('the cold learning return grants no access to a nonconfigured operator origin (%s, %s)', async (policy, origin) => {
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue('nu54://market-learning');
  mockInstalledPolicy.mockResolvedValue(policy);
  mockNativeOrigin.mockResolvedValue(origin);
  let view!: ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {
    view = ReactTestRenderer.create(<App />);
  });
  const snapshot = JSON.stringify(view.toJSON());
  expect(snapshot).toContain('테스트 자산으로 연습하기');
  expect(snapshot).toContain('StableNet 테스트넷 · 테스트 자산만');
  expect(snapshot).toContain('격리 테스트 체인 8283');
  const operator = view.root.findAll(node => node.props.accessibilityLabel === '시장 운영 모니터' &&
    typeof node.props.onPress === 'function');
  expect(operator).toHaveLength(1);
  expect(operator[0].props.accessibilityState.disabled).toBe(true);
  await ReactTestRenderer.act(async () => view.unmount());
});

test('learning home explains the test-only venue and offers real destinations before enrollment', async () => {
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  const open = jest.spyOn(Linking, 'openURL').mockResolvedValue(undefined).mockClear();
  mockNativeOrigin.mockResolvedValue('');
  let view!: ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => { view = ReactTestRenderer.create(<App />); });
  const buttons = (label: string) => view.root.findAll(node =>
    node.props.accessibilityLabel === label && typeof node.props.onPress === 'function');
  const tell = (text: string) => expect(JSON.stringify(view.toJSON())).toContain(text);
  try {
    // The learning market opens from the 둘러보기 tab.
    await ReactTestRenderer.act(async () => view.root.find(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '둘러보기').props.onPress());
    await ReactTestRenderer.act(async () => buttons('테스트 시장 학습')[0].props.onPress());
    tell('테스트 자산으로 연습하기');
    tell('StableNet 테스트넷 · 테스트 자산만');
    tell('실물 자산이나 실거래가 아닙니다.');
    for (const label of ['교환', '유동성', '모의 환전', '포지션', '여행']) {
      expect(buttons(label)).toHaveLength(1);
      expect(buttons(label)[0].props.accessibilityState.disabled).toBe(false);
    }
    expect(buttons('시장 운영 모니터')[0].props.accessibilityState.disabled).toBe(true);
    await ReactTestRenderer.act(async () => buttons('시장 운영 모니터')[0].props.onPress());
    await ReactTestRenderer.act(async () => buttons('모의 환전')[0].props.onPress());
    tell('모의 환전 연습');
    expect(buttons('견적 받기')).toHaveLength(0);
    await ReactTestRenderer.act(async () => buttons('여행')[0].props.onPress());
    tell('여행지 찾기');
    const returnToMarket = buttons('학습 시장으로 돌아가기')
      .find(node => node.props.accessibilityState?.disabled === false);
    expect(returnToMarket).toBeDefined();
    await ReactTestRenderer.act(async () => returnToMarket!.props.onPress());
    tell('StableNet 테스트넷 · 테스트 자산만');
    tell('모의 환전 연습');
    expect(open).not.toHaveBeenCalled();
  } finally {
    await ReactTestRenderer.act(async () => view.unmount());
  }
});
