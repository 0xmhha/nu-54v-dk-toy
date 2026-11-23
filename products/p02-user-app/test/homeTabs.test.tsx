import React from 'react';
import Renderer from 'react-test-renderer';
import {Platform} from 'react-native';
import App from '../src/App';
import {OrderPayment, type OrderPaymentView} from '../src/merchant/OrderPayment';
import {demoPayments} from '../src/wallet/DemoWalletHome';

let mockStored = '';
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => '', scan: async () => {}, stopScan: async () => {},
  disconnect: async () => {}, onScan: () => ({remove() {}})}}));
const mockProtect = jest.fn(async (_on: boolean) => {});
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => mockStored, setHomeProtection: (on: boolean) => mockProtect(on)}}));

let tree!: Renderer.ReactTestRenderer;
const text = () => JSON.stringify(tree.toJSON());
const pick = (label: string, role?: string) => tree.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function' &&
  (!role || node.props.accessibilityRole === role))[0];
const tap = (label: string, role?: string) => Renderer.act(async () => { pick(label, role).props.onPress(); });
const open = () => Renderer.act(async () => { tree = Renderer.create(<App />); });
const selected = () => [...new Set(tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityState?.selected)
  .map(node => node.props.accessibilityLabel))];
afterEach(async () => { await Renderer.act(async () => tree.unmount()); });

const personal = JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: 'ab'.repeat(32), address: '0x' + '12'.repeat(20), generation: 3, network: '8283'}});

test('four tabs, and settings behind the gear on every tab root', async () => {
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  mockStored = personal;
  await open();
  expect([...new Set(tree.root.findAll(node => node.props.accessibilityRole === 'tab').map(node => node.props.accessibilityLabel))])
    .toEqual(['홈', '결제', '지갑', '둘러보기']);
  expect(selected()).toEqual(['홈']);
  expect(mockProtect).toHaveBeenLastCalledWith(true);
  for (const tab of ['홈', '결제', '지갑', '둘러보기']) {
    await tap(tab, 'tab');
    expect(selected()).toEqual([tab]);
    expect([tab, !!pick('설정', 'button')]).toEqual([tab, true]);
  }
  await tap('설정', 'button');
  expect(text()).toContain('보안과 복구');
  expect(selected()).toEqual([]);
});

test('내 보드 opens from the home card and from settings, outside the tabs', async () => {
  mockStored = personal;
  await open();
  await tap('내 보드, 보드 상태를 아직 확인하지 않았습니다', 'button');
  expect(text()).toContain('LED 신호 보는 법');
  expect(selected()).toEqual([]);
  await tap('홈', 'tab');
  await tap('설정', 'button');
  await tap('보드 상태와 LED 신호', 'button');
  expect(text()).toContain('LED 신호 보는 법');
});

test('travel, booking, market, shops and wallet pages stay reachable', async () => {
  mockStored = personal;
  await open();
  for (const label of ['매장 QR 결제', '근처 매장', '여행 예약']) expect(pick(label, 'button')).toBeDefined();
  await tap('지갑', 'tab');
  for (const label of ['다른 체인 받기', '다른 체인 보내기', '개인 전송', '지급 기록 관리']) expect(pick(label, 'button')).toBeDefined();
  await tap('둘러보기', 'tab');
  for (const label of ['여행지 찾기', '가까운 매장에서 주문', 'DApp 둘러보기', '외부 DApp 연결', '온체인 토큰 권한', '테스트 시장 학습'])
    expect(pick(label, 'button')).toBeDefined();
});

test('before a board is connected home explains the three steps and hides the cloud wallet', async () => {
  mockStored = '';
  await open();
  expect(text()).toContain('내 하드월렛을 연결해 주세요');
  expect(text()).toContain('SW4를 길게 누르고');
  expect(text()).not.toContain('클라우드 지갑');
  await tap('보드 연결 시작', 'button');
  expect(text()).toContain('보드 연결하기');
});

describe('order payment states', () => {
  const actions = {findBoard: jest.fn(), checkBoard: jest.fn(), approve: jest.fn(), cancel: jest.fn(), startNew: jest.fn(), again: jest.fn(), openBoard: jest.fn(),
    check: jest.fn(), resend: jest.fn(), close: jest.fn()};
  const now = Math.floor(Date.now() / 1000);
  const draw = (view: OrderPaymentView) => Renderer.act(async () => { tree = Renderer.create(<OrderPayment view={view} actions={actions} />); });
  const expected: Record<string, [string, string[], string[]]> = {
    '결제 확인': ['그린 카페', ['보드로 승인', '결제 취소'], []],
    '보드 찾기 전': ['4.50 TestUSDC', ['내 보드 확인', '결제 취소'], ['보드로 승인']],
    '보드 승인 대기': ['승인할 수 있는 시간', ['승인 취소'], ['보드로 승인', '같은 결제 다시 보내기']],
    '잔액 부족': ['보드 계정의 잔액이 부족합니다', ['보드 계정 보기', '결제 취소'], ['보드로 승인']],
    '한도 초과': ['보드 한도를 넘는 금액입니다', ['결제 취소'], ['보드로 승인']],
    '오늘 한도 초과': ['오늘 남은 한도를 넘는 금액입니다', ['결제 취소'], ['보드로 승인']],
    '지원하지 않는 주문': ['이 매장에서는 아직', ['결제 취소'], ['보드로 승인', '결제 내용 다시 받기']],
    '승인 뒤 취소': ['오늘 남은 한도가 4.50 TestUSDC만큼 줄었습니다', ['결제 내용 다시 받기'], ['보드로 승인']],
    '결제 거절': ['빠져나간 돈은 없습니다', ['결제 내용 다시 받기'], ['보드로 승인']],
    '결제 보내는 중': ['같은 주문을 이어서 확인합니다', ['결제 화면 닫기'], ['보드로 승인', '결제 내용 다시 받기']],
    '결과 확인 중': ['다시 결제하지 마세요', ['결과 다시 확인', '같은 결제 다시 보내기'], ['보드로 승인', '결제 내용 다시 받기']],
    '결제 완료': ['그린 카페 주문번호 17', ['확인'], ['보드로 승인']],
    '결제 안 됨': ['빠져나간 돈이 없습니다', ['결제 화면 닫기'], ['보드로 승인']],
  };
  test.each(demoPayments(now))('%s', async (label, view) => {
    await draw(view);
    const [shown, present, absent] = expected[label];
    expect(text()).toContain(shown);
    for (const name of present) expect([name, !!pick(name)]).toEqual([name, true]);
    for (const name of absent) expect([name, !!pick(name)]).toEqual([name, false]);
    expect(text()).not.toMatch(/0x[0-9a-f]{6}|독립 표시/);
  });
  test('the board guidance names the LED, SW1 and the PIN', async () => {
    await draw({kind: 'waiting', store: '그린 카페', amount: '4.50 TestUSDC', deadline: now + 84});
    for (const word of ['빠르게 깜박임', 'SW1', 'SW2', 'PIN', '1:2']) expect(text()).toContain(word);
  });
  test('a page without a deadline draws no countdown', async () => {
    await draw({kind: 'confirm', store: '그린 카페', amount: '4.50 TestUSDC', items: [], block: 'unsupported'});
    expect(text()).not.toContain('결제 기한');
    expect(text()).not.toContain('0:00');
  });
  test('a refused order offers a new try only after its quote ends', async () => {
    await draw({kind: 'stopped', reason: 'refused', store: '그린 카페', amount: '4.50 TestUSDC', until: now + 60});
    expect(pick('결제 내용 다시 받기').props.disabled).toBe(true);
    expect(text()).toContain('다시 확인할 때까지');
    await Renderer.act(async () => tree.unmount());
    await draw({kind: 'stopped', reason: 'refused', store: '그린 카페', amount: '4.50 TestUSDC', until: now - 1});
    expect(pick('결제 내용 다시 받기').props.disabled).toBe(false);
  });
  test('the confirm page stops offering approval when its time is up', async () => {
    await draw({kind: 'confirm', store: '그린 카페', amount: '4.50 TestUSDC', deadline: now - 1, items: [], board: {perPayment: 'a', left: 'b'}});
    expect(pick('보드로 승인')).toBeUndefined();
    await tap('결제 내용 다시 받기');
    expect(actions.again).toHaveBeenCalledTimes(1);
  });
});
