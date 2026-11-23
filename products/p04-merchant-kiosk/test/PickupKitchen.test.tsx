import { AccessibilityInfo } from 'react-native';
import Renderer from 'react-test-renderer';
import { readPickupKitchen } from '@nu54/protocol';
import { PickupKitchen } from '../src/kiosk/PickupKitchen.tsx';
import { pickupHash } from '../src/kiosk/pickupJournal.ts';

const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const actor = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002';
const sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003';
const scope = (site: string, person: string, slot = 'terminal') =>
  'pickup-v2-' + pickupHash([site, person, slot].join('/'));
const dispatch = (raw: string) => mockExchange('terminal', raw);
const mockExchange = jest.fn();
const mockJournals = new Map<string, string>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadJournal: async (name: string) => mockJournals.get(name) ?? '',
    saveJournal: async (name: string, value: string) => {
      mockJournals.set(name, value);
    },
    randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
    exchange: (...args: unknown[]) => mockExchange(...args),
  },
}));

test('retries a lost pickup state response with the original terminal command', async () => {
  let replyLost = true;
  let state = 'accepted';
  let revision = 0;
  const commands: string[] = [];
  mockJournals.clear();
  mockExchange.mockImplementation(async (slot, raw) => {
    expect(slot).toBe('terminal');
    const { travel } = JSON.parse(raw);
    expect(travel.branch).toBe(branch);
    if (travel.operation === 'pickup-change') {
      commands.push(raw);
      if (replyLost) {
        replyLost = false;
        state = travel.target;
        revision = travel.expectedRevision + 1;
        throw new Error('lost response');
      }
      return JSON.stringify({ revision });
    }
    return JSON.stringify({
      pickups: [
        {
          saleRef: sale,
          startsAt: '2027-01-02T09:00:00Z',
          timezone: 'Asia/Seoul',
          state,
          pickupNo: 8,
          revision,
        },
      ],
    });
  });
  async function open() {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(
        <PickupKitchen
          journal={scope(branch, actor)}
          transport={dispatch}
          branch={branch}
          actor={actor}
          close={() => {}}
        />,
      );
    });
    return tree;
  }
  async function press(tree: Renderer.ReactTestRenderer, pattern: RegExp) {
    const button = tree.root.findAll(
      node =>
        typeof node.props.onPress === 'function' &&
        pattern.test(node.props.accessibilityLabel ?? ''),
    )[0];
    expect(button).toBeDefined();
    await Renderer.act(async () => button.props.onPress());
  }

  let tree = await open();
  await press(tree, /^주문 목록 새로 고침$/);
  await press(tree, /^픽업 8 ·/);
  await press(tree, /^조리 시작$/);
  expect([...mockJournals.values()].join('')).toContain(sale);
  await Renderer.act(async () => tree.unmount());

  tree = await open();
  await press(tree, /^주문 목록 새로 고침$/);
  await press(tree, /^방금 누른 처리 결과 확인$/);
  expect(commands).toHaveLength(2);
  expect(commands[0]).toBe(commands[1]);
  expect([...mockJournals.values()].join('')).not.toContain(sale);
  expect(JSON.stringify(tree.toJSON())).toContain('준비 중');
  await Renderer.act(async () => tree.unmount());
});

test('an inherited v1 pickup receipt does not lock another authenticated clerk or branch out of its queue', async () => {
  const other = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
  const otherBranch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005';
  const legacy = JSON.stringify({
    branch,
    actor,
    sale,
    requestKey: '0fbc54ef-2827-4ab5-bfcd-3a27611c3006',
    expectedRevision: 0,
    target: 'preparing',
  });
  mockJournals.clear();
  mockJournals.set('travel-pickup-kitchen-v1', legacy);
  mockExchange.mockReset().mockResolvedValue(JSON.stringify({ pickups: [] }));
  for (const [allowedBranch, staff] of [
    [branch, other],
    [otherBranch, actor],
  ]) {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(
        <PickupKitchen
          journal={scope(allowedBranch, staff)}
          transport={dispatch}
          branch={allowedBranch}
          actor={staff}
          close={() => {}}
        />,
      );
    });
    const query = tree.root.findAll(
      node =>
        node.props.accessibilityLabel === '주문 목록 새로 고침' &&
        typeof node.props.onPress === 'function',
    )[0];
    await Renderer.act(async () => query.props.onPress());
    expect(JSON.stringify(tree.toJSON())).toContain('접수된 픽업이 없습니다.');
    expect(JSON.stringify(tree.toJSON())).not.toContain(
      '방금 누른 처리 결과 확인',
    );
    expect(mockJournals.get('travel-pickup-kitchen-v1')).toBe(legacy);
    await Renderer.act(async () => tree.unmount());
  }
  // Each clerk: the automatic read when the screen opens and the manual one.
  expect(mockExchange).toHaveBeenCalledTimes(4);
});

test('a legacy receipt is adopted once, a foreign role cannot clear it, and the completed receipt is not resurrected', async () => {
  const owner = scope(branch, actor);
  const foreign = scope(branch, actor, 'operator');
  const pending = {
    branch,
    actor,
    sale,
    requestKey: '0fbc54ef-2827-4ab5-bfcd-3a27611c3006',
    expectedRevision: 0,
    target: 'preparing',
  };
  const legacy = JSON.stringify(pending);
  mockJournals.clear();
  mockJournals.set('travel-pickup-kitchen-v1', legacy);
  const commands: string[] = [];
  mockExchange.mockReset().mockImplementation(async (_slot, raw) => {
    const { travel } = JSON.parse(raw);
    if (travel.operation === 'pickup-change') {
      commands.push(travel.requestKey);
      return JSON.stringify({ revision: 1 });
    }
    return JSON.stringify({
      pickups: [
        {
          saleRef: sale,
          startsAt: '2027-01-02T09:00:00Z',
          timezone: 'Asia/Seoul',
          state: 'preparing',
          pickupNo: 8,
          revision: 1,
        },
      ],
    });
  });
  async function open(journal: string) {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(
        <PickupKitchen
          journal={journal}
          transport={dispatch}
          branch={branch}
          actor={actor}
          close={() => {}}
        />,
      );
    });
    const queue = tree.root.findAll(
      node =>
        node.props.accessibilityLabel === '주문 목록 새로 고침' &&
        typeof node.props.onPress === 'function',
    )[0];
    await Renderer.act(async () => queue.props.onPress());
    return tree;
  }
  const resume = '방금 누른 처리 결과 확인';
  const adopted = await open(owner);
  expect(mockJournals.get(owner)).toBe(legacy);
  await Renderer.act(async () => adopted.unmount());
  const other = await open(foreign);
  expect(JSON.stringify(other.toJSON())).not.toContain(resume);
  expect(mockJournals.get(owner)).toBe(legacy);
  await Renderer.act(async () => other.unmount());
  const original = await open(owner);
  const retry = original.root.findAll(
    node =>
      node.props.accessibilityLabel === resume &&
      typeof node.props.onPress === 'function',
  )[0];
  await Renderer.act(async () => retry.props.onPress());
  expect(commands).toEqual([pending.requestKey]);
  expect(mockJournals.get(owner)).toBe('null');
  expect(mockJournals.get('travel-pickup-kitchen-v1')).toBe(legacy);
  await Renderer.act(async () => original.unmount());
  const closed = await open(owner);
  expect(JSON.stringify(closed.toJSON())).not.toContain(resume);
  await Renderer.act(async () => closed.unmount());
});

test('a new clerk in the same branch can act without overwriting the original clerk’s pending command', async () => {
  const replacement = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
  const replacementSale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005';
  const owner = scope(branch, actor);
  const successor = scope(branch, replacement);
  const receipt = JSON.stringify({
    branch,
    actor,
    sale,
    requestKey: '0fbc54ef-2827-4ab5-bfcd-3a27611c3006',
    expectedRevision: 0,
    target: 'preparing',
  });
  mockJournals.clear();
  mockJournals.set(owner, receipt);
  let revision = 0;
  const transport = jest.fn(async raw => {
    const { travel } = JSON.parse(raw);
    if (travel.operation === 'pickup-change') {
      expect(travel.sale).toBe(replacementSale);
      expect(travel.requestKey).not.toBe(JSON.parse(receipt).requestKey);
      revision++;
      return JSON.stringify({ revision });
    }
    return JSON.stringify({
      pickups: [
        {
          saleRef: replacementSale,
          startsAt: '2027-01-02T09:00:00Z',
          timezone: 'Asia/Seoul',
          pickupNo: 9,
          state: revision ? 'preparing' : 'accepted',
          revision,
        },
      ],
    });
  });
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(
      <PickupKitchen
        journal={successor}
        transport={transport}
        branch={branch}
        actor={replacement}
        close={() => {}}
      />,
    );
  });
  async function press(pattern: RegExp) {
    const button = tree.root.findAll(
      node =>
        typeof node.props.onPress === 'function' &&
        pattern.test(node.props.accessibilityLabel ?? ''),
    )[0];
    expect(button).toBeDefined();
    await Renderer.act(async () => button.props.onPress());
  }
  await press(/^주문 목록 새로 고침$/);
  expect(JSON.stringify(tree.toJSON())).not.toContain(
    '방금 누른 처리 결과 확인',
  );
  await press(/^픽업 9 ·/);
  await press(/^조리 시작$/);
  expect(revision).toBe(1);
  expect(mockJournals.get(successor)).toBe('null');
  expect(mockJournals.get(owner)).toBe(receipt);
  await Renderer.act(async () => tree.unmount());
});

test('a cafe order awaiting fulfillment has no pickup or print action and reopens the same original order', async () => {
  const event = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
  let stage = 'checking';
  const transport = jest.fn(async (_wire: string) =>
    JSON.stringify({
      pickups: [
        {
          saleRef: sale,
          kind: 'cafe',
          startsAt: '2027-01-02T09:00:00Z',
          timezone: 'Asia/Seoul',
          state: stage,
          revision: stage === 'checking' ? 0 : 2,
          ...(stage === 'checking' ? {} : { pickupNo: 17, eventRef: event }),
        },
      ],
    }),
  );
  const openReceipt = jest.fn();
  let tree!: Renderer.ReactTestRenderer;
  const launch = async () => Renderer.act(async () => {
    tree = Renderer.create(
      <PickupKitchen
        branch={branch}
        actor={actor}
        journal={scope(branch, actor, 'cafe')}
        transport={transport}
        allowCafe
        openReceipt={openReceipt}
        close={() => {}}
      />,
    );
  });
  const find = (label: string | RegExp) =>
    tree.root.findAll(
      node =>
        node.props.accessibilityRole === 'button' &&
        (typeof label === 'string'
          ? node.props.accessibilityLabel === label
          : label.test(node.props.accessibilityLabel ?? '')),
    )[0];
  const press = async (label: string | RegExp) => {
    await Renderer.act(async () => find(label).props.onPress());
  };
  const firstRead = Date.parse('2027-01-02T09:00:00Z');
  const clock = jest.spyOn(Date, 'now').mockReturnValue(firstRead);
  const readTime = (time: number) =>
    new Date(time).toLocaleTimeString('ko-KR', {hour: '2-digit', minute: '2-digit'});
  try {
    await launch();
    await press('주문 목록 새로 고침');
    await press(/^확인 중 주문 /);
    expect(JSON.stringify(tree.toJSON())).toContain(readTime(firstRead));
    await Renderer.act(async () => tree.unmount());
    await launch();
    expect(JSON.stringify(tree.toJSON())).not.toContain('마지막 확인');
    await press('주문 목록 새로 고침');
    await press(/^확인 중 주문 /);
    clock.mockReturnValue(firstRead + 60_000);
    await press('결제 결과 다시 확인');
    expect(JSON.stringify(tree.toJSON())).toContain(readTime(firstRead + 60_000));
    expect(JSON.stringify(tree.toJSON())).not.toContain(`"마지막 확인 ","${readTime(firstRead)}"`);
    expect(JSON.stringify(tree.toJSON())).not.toContain(sale);
    await press('진단 정보');
    expect(JSON.stringify(tree.toJSON())).toContain(sale);
    expect(JSON.stringify(tree.toJSON())).not.toContain(event);
    expect(find('조리 시작')).toBeUndefined();
    expect(find('영수증 보기')).toBeUndefined();
    stage = 'ready';
    await press('결제 결과 다시 확인');
    expect(find('전달 완료')).toBeDefined();
    expect(JSON.stringify(tree.toJSON())).toContain(event);
    expect(
      JSON.parse(transport.mock.calls.at(-1)?.[0] ?? '{}').travel,
    ).toMatchObject({ operation: 'pickup-detail', sale });
    await press('영수증 보기');
    expect(openReceipt).toHaveBeenCalledWith(sale);
    expect(transport.mock.calls.map(([raw]) => JSON.parse(raw).travel.operation))
      .toEqual(['pickup-queue', 'pickup-queue', 'pickup-detail', 'pickup-queue', 'pickup-queue', 'pickup-detail', 'pickup-detail', 'pickup-detail']);
  } finally {
    clock.mockRestore();
    await Renderer.act(async () => tree?.unmount());
  }
});

test('K05 separates the nearby ready detail from its public preparing order', async () => {
  const publicSale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3006';
  const nearbyEvent = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
  const publicEvent = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005';
  const asset = '0x' + 'b'.repeat(40);
  const local = 'Asia/Seoul';
  const expected = {finalizedAt: '2030-10-19T01:40:00.000000Z', pickupReadyAt: '2030-10-19T01:41:00.000000Z'};
  const base = {startsAt: '2030-10-19T01:39:00Z', timezone: local, kind: 'cafe'};
  const nearby = {...base, saleRef: sale, state: 'ready', revision: 2, pickupNo: 16, eventRef: nearbyEvent};
  const publicOrder = {...base, saleRef: publicSale, state: 'preparing', revision: 1, pickupNo: 17, eventRef: publicEvent};
  let nearbyZone = 'invalid/zone';
  const transport = jest.fn(async (raw: string) => {
    const {travel} = JSON.parse(raw);
    expect(travel.branch).toBe(branch);
    return JSON.stringify({pickups: travel.operation === 'pickup-queue' ? [publicOrder, nearby]
      : travel.sale === sale ? [{...nearby, timezone: nearbyZone, settlement: {...expected, amount: '8000000', asset}}] : [publicOrder]});
  });
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<PickupKitchen branch={branch} actor={actor} allowCafe transport={transport}
      journal={scope(branch, actor, 'cafe')} close={() => {}} paymentProfile={{
        chain: 8283, asset, contract: '0x' + 'c'.repeat(40), beneficiary: '0x' + 'd'.repeat(40),
        signer: '0x' + 'e'.repeat(40), merchantId: 'a'.repeat(64), keyVersion: 1, beneficiaryVersion: 1,
        maxBaseUnits: '30000000',
      }}/>);
  });
  const display = () => JSON.stringify(tree.toJSON());
  const press = async (label: RegExp) => {
    const [button] = tree.root.findAll(node => node.props.accessibilityRole === 'button' && label.test(node.props.accessibilityLabel ?? ''));
    await Renderer.act(async () => button.props.onPress());
  };
  try {
    await press(/^주문 목록 새로 고침$/);
    expect(display()).not.toContain('결제 금액');
    await press(/^픽업 16 /);
    expect(display()).toContain('직원 권한과 연결을 확인하고 다시 시도하세요.');
    expect(display()).not.toContain(nearbyEvent);
    expect(display()).not.toContain('전달 완료');
    expect(display()).not.toContain('픽업 가능 시각');
    nearbyZone = local;
    await press(/^주문 목록 새로 고침$/);
    await press(/^픽업 16 /);
    expect(display()).not.toContain(nearbyEvent);
    await press(/^진단 정보$/);
    expect(display()).toContain(nearbyEvent);
    expect(display()).not.toContain(publicEvent);
    expect(display()).toContain('8.00 TestUSDC');
    expect(display()).toContain('결제 확정 시각');
    expect(display()).toContain('픽업 가능 시각');
    const formatter = new Intl.DateTimeFormat('ko-KR', {dateStyle: 'medium', timeStyle: 'short', timeZone: local});
    expect(display()).toContain(formatter.format(new Date(expected.finalizedAt)));
    expect(display()).toContain(formatter.format(new Date(expected.pickupReadyAt)));
    expect(display()).not.toContain(expected.pickupReadyAt);
    await press(/^픽업 17 /);
    expect(display()).toContain(publicEvent);
    expect(display()).not.toContain(nearbyEvent);
    expect(display()).not.toContain('결제 확정 시각');
    expect(transport.mock.calls.map(([raw]) => {
      const {travel} = JSON.parse(raw);
      return [travel.operation, travel.sale];
    })).toEqual([['pickup-queue', undefined], ['pickup-queue', undefined], ['pickup-detail', sale], ['pickup-queue', undefined],
      ['pickup-detail', sale], ['pickup-detail', publicSale]]);
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

test('the queue refreshes itself, groups orders by stage, announces a new order once and keeps the list when a read fails', async () => {
  jest.useFakeTimers({ doNotFake: ['setImmediate', 'clearImmediate'] });
  const announce = jest.spyOn(AccessibilityInfo, 'announceForAccessibility').mockImplementation(() => {});
  const ref = (n: number) => `0fbc54ef-2827-4ab5-bfcd-3a27611c40${n}`;
  const row = (n: number, state: string) => ({
    saleRef: ref(n), startsAt: '2027-01-02T09:00:00Z', timezone: 'Asia/Seoul', pickupNo: n, state, revision: 1,
  });
  let pickups = [row(16, 'ready'), row(17, 'preparing')];
  let online = true;
  const transport = jest.fn(async () => {
    if (!online) throw new Error('offline');
    return JSON.stringify({ pickups });
  });
  let tree!: Renderer.ReactTestRenderer;
  const text = () => JSON.stringify(tree.toJSON());
  const pass = (ms: number) => Renderer.act(async () => { await jest.advanceTimersByTimeAsync(ms); });
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(
        <PickupKitchen journal={scope(branch, actor, 'refresh')} transport={transport} branch={branch} actor={actor} close={() => {}} />,
      );
    });
    expect(text()).toContain('"준비 중"');
    expect(text()).toContain('픽업 17');
    expect(announce).not.toHaveBeenCalled();
    pickups = [row(18, 'accepted'), ...pickups];
    await pass(5_000);
    expect(text()).toContain('픽업 18');
    expect(text()).toContain('새 주문 1건');
    await pass(5_000);
    expect(announce.mock.calls).toEqual([['새 주문 1건이 들어왔습니다']]);
    online = false;
    await pass(5_000);
    expect(text()).toContain('새 주문을 확인하지 못하고 있습니다');
    expect(text()).toContain('픽업 18');
    online = true;
    await pass(5_000);
    expect(text()).not.toContain('새 주문을 확인하지 못하고 있습니다');
    await Renderer.act(async () => tree.unmount());
    const reads = transport.mock.calls.length;
    await pass(30_000);
    expect(transport).toHaveBeenCalledTimes(reads);
  } finally {
    announce.mockRestore();
    jest.useRealTimers();
  }
});

describe('card actions', () => {
  const at = { startsAt: '2027-01-02T09:00:00Z', timezone: 'Asia/Seoul', kind: 'cafe' };
  const ref = (n: number) => `0fbc54ef-2827-4ab5-bfcd-3a27611c40${n}`;
  type Row = { saleRef: string; state: string; pickupNo?: number; revision: number; eventRef?: string; items?: object[]; escrow?: { stage: string } };
  let rows: Row[];
  let commands: { sale: string; target: string; expectedRevision: number; requestKey: string }[];
  let tree: Renderer.ReactTestRenderer;
  const text = () => JSON.stringify(tree.toJSON());
  const find = (label: string | RegExp) => tree.root.findAll(node => node.props.accessibilityRole === 'button' &&
    typeof node.props.onPress === 'function' &&
    (typeof label === 'string' ? node.props.accessibilityLabel === label : label.test(node.props.accessibilityLabel ?? '')))[0];
  const press = async (label: string | RegExp) => {
    expect(find(label)).toBeDefined();
    expect(find(label).props.disabled).toBeFalsy();
    await Renderer.act(async () => find(label).props.onPress());
  };
  const row = (n: number, state: string, more: Partial<Row> = {}): Row =>
    ({ saleRef: ref(n), state, pickupNo: n, eventRef: ref(n + 20), revision: 1, ...more });
  const locked = { escrow: { stage: 'locked' } };
  const order = [{ label: '아메리카노', option: '아이스', quantity: 2 }, { label: '크루아상', option: 'standard', quantity: 1 }];
  async function mount() {
    await Renderer.act(async () => {
      tree = Renderer.create(
        <PickupKitchen journal={scope(branch, actor, 'cards')} branch={branch} actor={actor} allowCafe
          openReceipt={() => {}} close={() => {}}
          transport={async raw => {
            const { travel } = JSON.parse(raw);
            if (travel.operation === 'pickup-change') {
              const { sale: saleRef, target, expectedRevision, requestKey } = travel;
              commands.push({ sale: saleRef, target, expectedRevision, requestKey });
              rows = rows.map(item => item.saleRef === saleRef ? { ...item, state: target, revision: item.revision + 1 } : item);
              return JSON.stringify({ revision: expectedRevision + 1 });
            }
            const shown = travel.operation === 'pickup-detail' ? rows.filter(item => item.saleRef === travel.sale)
              : rows.filter(item => item.state !== 'unfulfillable');
            return JSON.stringify({ pickups: shown.map(item => ({ ...at, ...item })) });
          }} />,
      );
    });
  }
  beforeEach(() => {
    mockJournals.clear();
    commands = [];
  });
  afterEach(async () => Renderer.act(async () => tree.unmount()));

  test('a card names the items and moves an order to pickup without opening it or showing an id', async () => {
    rows = [row(18, 'accepted', { items: order }), { saleRef: ref(40), state: 'checking', revision: 0 }];
    await mount();
    expect(text()).toContain('아메리카노 아이스 2 · 크루아상 1');
    expect(text()).toContain('확인 중 주문');
    await press('조리 시작');
    await press('준비 완료 · 고객에게 알림');
    expect(commands.map(({ sale: saleRef, target, expectedRevision }) => [saleRef, target, expectedRevision]))
      .toEqual([[ref(18), 'preparing', 1], [ref(18), 'ready', 2]]);
    expect(new Set(commands.map(item => item.requestKey)).size).toBeLessThanOrEqual(2);
    for (const id of [ref(18), ref(38), ref(40)]) expect(text()).not.toContain(id);
    expect([...mockJournals.values()]).toEqual(['null']);
  });

  test('a queue without items keeps working', async () => {
    rows = [row(18, 'preparing')];
    await mount();
    await press('준비 완료 · 고객에게 알림');
    expect(commands).toHaveLength(1);
  });

  test('handing over an escrow order asks once, says what it starts, and can be left', async () => {
    rows = [row(16, 'ready', { items: order, ...locked })];
    await mount();
    await press('전달 완료');
    expect(commands).toHaveLength(0);
    expect(text()).toContain('구매자의 확인 기간이 시작됩니다.');
    expect(text()).toContain('되돌릴 수 없습니다.');
    await press('확정하지 않기');
    expect(text()).not.toContain('구매자의 확인 기간이 시작됩니다.');
    await press('전달 완료');
    await press('전달 완료 확정');
    expect(commands.map(item => item.target)).toEqual(['collected']);
    expect(text()).toContain('전달한 주문');
  });

  test('refusing an escrow order is offered only in its detail and says the money goes back', async () => {
    rows = [row(17, 'preparing', locked)];
    await mount();
    expect(find('이행 불가 · 환불')).toBeUndefined();
    await press(/^픽업 17 /);
    await press('이행 불가 · 환불');
    expect(text()).toContain('결제 금액은 구매자에게 자동으로 돌아갑니다.');
    expect(commands).toHaveLength(0);
    await press('이행 불가 · 환불 확정');
    expect(commands.map(item => item.target)).toEqual(['unfulfillable']);
  });

  test('an order without an escrow keeps the earlier refund text', async () => {
    rows = [row(16, 'ready')];
    await mount();
    await press('전달 완료');
    expect(text()).not.toContain('확인 기간');
    await press('확정하지 않기');
    await press(/^픽업 16 /);
    await press('이행 불가 · 환불');
    expect(text()).toContain('재무 담당이 따로 심사합니다.');
    expect(text()).not.toContain('자동으로 돌아갑니다');
  });

  test('orders whose money has moved leave the working columns, disputes first, and offer no action', async () => {
    rows = [row(14, 'ready', { escrow: { stage: 'paid' } }), row(15, 'ready', { escrow: { stage: 'refunded' } }),
      row(16, 'collected', { escrow: { stage: 'disputed' } }), row(17, 'preparing', locked), row(19, 'accepted', { escrow: { stage: 'refunded' } })];
    await mount();
    const shown = text();
    expect(shown.indexOf('처리 끝남·확인 필요')).toBeGreaterThan(shown.indexOf('픽업 대기'));
    expect(shown.match(/이의 접수|환불됨|대금 수령/g)).toEqual(['이의 접수', '환불됨', '환불됨', '대금 수령']);
    for (const stage of ['refunded', 'disputed', 'locked']) expect(shown).not.toContain(`"${stage}"`);
    expect(tree.root.findAll(node => node.props.accessibilityLabel === '픽업 대기 0건').length).toBeGreaterThan(0);
    expect(tree.root.findAll(node => node.props.accessibilityLabel === '새 주문 0건').length).toBeGreaterThan(0);
    for (const label of ['전달 완료', '조리 시작']) expect(find(label)).toBeUndefined();
    expect(find('준비 완료 · 고객에게 알림').props.disabled).toBe(false);
    expect(shown).toContain('물건을 건네지 마세요.');
    await press(/^픽업 15 /);
    expect(find('이행 불가 · 환불')).toBeUndefined();
    expect(find('전달 완료')).toBeUndefined();
  });

  test('a collected order whose escrow is still locked asks nothing more of the clerk', async () => {
    rows = [row(16, 'collected', locked)];
    await mount();
    expect(text()).toContain('전달 완료로 기록했습니다. 더 누를 것은 없습니다.');
    await press(/^픽업 16 /);
    for (const label of ['전달 완료', '전달 완료 확정', '방금 누른 처리 결과 확인', '이행 불가 · 환불']) expect(find(label)).toBeUndefined();
    rows = [row(16, 'collected', { escrow: { stage: 'refunded' } })];
    await press('주문 목록 새로 고침');
    expect(text()).toContain('환불됨');
    expect(text()).not.toContain('물건을 건네지 마세요.');
  });

  test('ids and internal words stay out of the detail until the diagnostics are opened', async () => {
    rows = [row(17, 'preparing', { items: order, ...locked })];
    await mount();
    await press(/^픽업 17 /);
    for (const word of [ref(17), ref(37), '원주문', '이행 ID', '식별']) expect(text()).not.toContain(word);
    await press('진단 정보');
    expect(text()).toContain(ref(17));
    expect(text()).toContain(ref(37));
    await press('진단 정보 닫기');
    expect(text()).not.toContain(ref(17));
  });

  test('a question about an older revision disappears when the order changes elsewhere', async () => {
    rows = [row(16, 'ready')];
    await mount();
    await press('전달 완료');
    rows = [row(16, 'ready', { revision: 2 })];
    await press('주문 목록 새로 고침');
    expect(find('전달 완료 확정')).toBeUndefined();
    rows = [row(16, 'ready', { revision: 2, escrow: { stage: 'refunded' } })];
    await press('전달 완료');
    await press('주문 목록 새로 고침');
    expect(find('전달 완료 확정')).toBeUndefined();
    expect(commands).toHaveLength(0);
  });

  const unknown = { escrow: { stage: 'released' } };
  test('an escrow row with an unknown stage is refused by the shared reader and never shown', async () => {
    rows = [row(16, 'ready', unknown)];
    expect(() => readPickupKitchen(JSON.stringify({ pickups: rows.map(item => ({ ...at, ...item })) }))).toThrow('PICKUP_UNAVAILABLE');
    await mount();
    expect(find(/^픽업 16 /)).toBeUndefined();
    expect(text()).toContain('앱 업데이트가 필요합니다.');
    rows = [row(16, 'ready', locked)];
    await press('주문 목록 새로 고침');
    expect(text()).not.toContain('앱 업데이트가 필요합니다.');
    expect(find('전달 완료')).toBeDefined();
  });

  test('an unknown stage keeps the last readable list and says an update is needed', async () => {
    rows = [row(16, 'ready', locked), row(17, 'preparing', locked)];
    await mount();
    rows = [row(16, 'ready', unknown), row(17, 'preparing', locked), row(18, 'accepted', locked)];
    await press('주문 목록 새로 고침');
    expect(text()).toContain('앱 업데이트가 필요합니다.');
    expect(text()).toContain('새 주문이 빠져 있을 수 있습니다.');
    expect(find(/^픽업 16 /)).toBeDefined();
    expect(find(/^픽업 18 /)).toBeUndefined();
    // A readable order can still be worked on, and the kept list does not offer the finished step again.
    await press('준비 완료 · 고객에게 알림');
    expect(commands.map(item => item.target)).toEqual(['ready']);
    expect(find('준비 완료 · 고객에게 알림')).toBeUndefined();
    expect([...mockJournals.values()]).toEqual(['null']);
    expect(text()).toContain('앱 업데이트가 필요합니다.');
  });

  test('a command the server answered is finished even when its new stage is unreadable', async () => {
    rows = [row(16, 'ready', locked)];
    await mount();
    rows = [row(16, 'ready', unknown)];
    await press('전달 완료');
    await press('전달 완료 확정');
    expect(commands.map(item => item.target)).toEqual(['collected']);
    expect(find('방금 누른 처리 결과 확인')).toBeUndefined();
    expect(find('전달 완료')).toBeUndefined();
    expect(text()).toContain('전달 완료로 기록했습니다.');
    expect([...mockJournals.values()]).toEqual(['null']);
  });

  test('an answer that is wrong in another way is not called an update', async () => {
    rows = [row(16, 'ready', { ...unknown, pickupNo: -1 })];
    await mount();
    expect(text()).not.toContain('앱 업데이트가 필요합니다.');
    expect(text()).toContain('직원 권한과 연결을 확인하고 다시 시도하세요.');
  });
});
