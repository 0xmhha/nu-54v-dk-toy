import Renderer from 'react-test-renderer';
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
  await press(tree, /^지점 픽업 접수 조회$/);
  await press(tree, /^픽업 8 ·/);
  await press(tree, /^준비 시작$/);
  expect([...mockJournals.values()].join('')).toContain(sale);
  await Renderer.act(async () => tree.unmount());

  tree = await open();
  await press(tree, /^지점 픽업 접수 조회$/);
  await press(tree, /^같은 픽업 상태 요청 다시 확인$/);
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
        node.props.accessibilityLabel === '지점 픽업 접수 조회' &&
        typeof node.props.onPress === 'function',
    )[0];
    await Renderer.act(async () => query.props.onPress());
    expect(JSON.stringify(tree.toJSON())).toContain('접수된 픽업이 없습니다.');
    expect(JSON.stringify(tree.toJSON())).not.toContain(
      '같은 픽업 상태 요청 다시 확인',
    );
    expect(mockJournals.get('travel-pickup-kitchen-v1')).toBe(legacy);
    await Renderer.act(async () => tree.unmount());
  }
  expect(mockExchange).toHaveBeenCalledTimes(2);
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
        node.props.accessibilityLabel === '지점 픽업 접수 조회' &&
        typeof node.props.onPress === 'function',
    )[0];
    await Renderer.act(async () => queue.props.onPress());
    return tree;
  }
  const resume = '같은 픽업 상태 요청 다시 확인';
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
  await press(/^지점 픽업 접수 조회$/);
  expect(JSON.stringify(tree.toJSON())).not.toContain(
    '같은 픽업 상태 요청 다시 확인',
  );
  await press(/^픽업 9 ·/);
  await press(/^준비 시작$/);
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
    await press('지점 픽업 접수 조회');
    await press(/^확인 중 주문 /);
    expect(JSON.stringify(tree.toJSON())).toContain(readTime(firstRead));
    await Renderer.act(async () => tree.unmount());
    await launch();
    expect(JSON.stringify(tree.toJSON())).not.toContain(sale);
    await press('지점 픽업 접수 조회');
    await press(/^확인 중 주문 /);
    clock.mockReturnValue(firstRead + 60_000);
    await press('기존 지급 결과 재조회');
    expect(JSON.stringify(tree.toJSON())).toContain(readTime(firstRead + 60_000));
    expect(JSON.stringify(tree.toJSON())).not.toContain(readTime(firstRead));
    expect(JSON.stringify(tree.toJSON())).toContain(sale);
    expect(JSON.stringify(tree.toJSON())).not.toContain(event);
    expect(find('준비 시작')).toBeUndefined();
    expect(find('같은 원주문 영수증 열기')).toBeUndefined();
    stage = 'ready';
    await press('기존 지급 결과 재조회');
    expect(find('수령 완료 확인')).toBeDefined();
    expect(JSON.stringify(tree.toJSON())).toContain(event);
    expect(
      JSON.parse(transport.mock.calls.at(-1)?.[0] ?? '{}').travel,
    ).toMatchObject({ operation: 'pickup-detail', sale });
    await press('같은 원주문 영수증 열기');
    expect(openReceipt).toHaveBeenCalledWith(sale);
    expect(transport.mock.calls.map(([raw]) => JSON.parse(raw).travel.operation))
      .toEqual(['pickup-queue', 'pickup-detail', 'pickup-queue', 'pickup-detail', 'pickup-detail', 'pickup-detail']);
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
    await press(/^지점 픽업 접수 조회$/);
    expect(display()).not.toContain('지급 금액');
    await press(/^픽업 16 /);
    expect(display()).toContain('같은 요청을 다시 확인하세요.');
    expect(display()).not.toContain(nearbyEvent);
    expect(display()).not.toContain('수령 완료 확인');
    expect(display()).not.toContain('픽업 가능 시각');
    nearbyZone = local;
    await press(/^지점 픽업 접수 조회$/);
    await press(/^픽업 16 /);
    expect(display()).toContain(nearbyEvent);
    expect(display()).not.toContain(publicEvent);
    expect(display()).toContain('8.00 TestUSDC');
    expect(display()).toContain('지급 확정 시각');
    expect(display()).toContain('픽업 가능 시각');
    const formatter = new Intl.DateTimeFormat('ko-KR', {dateStyle: 'medium', timeStyle: 'short', timeZone: local});
    expect(display()).toContain(formatter.format(new Date(expected.finalizedAt)));
    expect(display()).toContain(formatter.format(new Date(expected.pickupReadyAt)));
    expect(display()).not.toContain(expected.pickupReadyAt);
    await press(/^픽업 17 /);
    expect(display()).toContain(publicEvent);
    expect(display()).not.toContain(nearbyEvent);
    expect(display()).not.toContain('지급 확정 시각');
    expect(transport.mock.calls.map(([raw]) => {
      const {travel} = JSON.parse(raw);
      return [travel.operation, travel.sale];
    })).toEqual([['pickup-queue', undefined], ['pickup-detail', sale], ['pickup-queue', undefined],
      ['pickup-detail', sale], ['pickup-detail', publicSale]]);
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});
