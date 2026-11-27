import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import { Text } from 'react-native';
import type { CommerceClient, SubscriptionInvoice } from '@nu54/protocol';
import Alerts from '../src/specs/NativeSubscriptionAlerts.ts';
import Http from '../src/specs/NativeCommerceHttp.ts';
import { isolateSubscriptionReminders, SubscriptionScreen } from '../src/merchant/subscriptions/SubscriptionScreen';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadJournal: jest.fn(async () => { throw new Error('device settings unavailable'); }),
    saveJournal: jest.fn(async () => undefined),
  },
}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({
  __esModule: true,
  default: {
    permission: jest.fn(async () => false),
    schedule: jest.fn(async () => false),
    remove: jest.fn(async () => undefined),
    clear: jest.fn(async () => undefined),
  },
}));

test('denied alerts and unreadable device settings keep the same unpaid invoice accessible', async () => {
  const row = {
    id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    saleRef: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
    cycle: '2026-11-01', dueAt: '2026-11-08T00:00:00Z',
    state: 'preview', amount: '30', asset: '0x' + '34'.repeat(20),
  } satisfies Partial<SubscriptionInvoice>;
  const client = {
    profile: { branchRef: 'branch', subjectRef: 'customer' },
    subscriptionOffers: async () => [],
    subscriptions: async () => [],
    invoices: async () => [row],
  } as unknown as CommerceClient;
  const onInvoice = jest.fn();
  let tree!: ReactTestRenderer.ReactTestRenderer;
  const press = async (label: string) => {
    const button = tree.root.findAll(node =>
      node.props.accessibilityLabel === label && typeof node.props.onPress === 'function',
    )[0];
    expect(button).toBeDefined();
    await ReactTestRenderer.act(async () => { button?.props.onPress(); });
  };
  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(
      <SubscriptionScreen client={client} keyFor={async () => 'id'} onInvoice={onInvoice} />,
    );
  });
  await press('회차 알림 허용');
  expect(tree.root.findAllByType(Text).map(node => node.props.children))
    .toContain('알림을 허용하지 않았습니다. 미결 목록은 사용할 수 있습니다.');
  await press('같은 회차 결제 검토');
  expect(onInvoice).toHaveBeenCalledTimes(1);
  expect(onInvoice).toHaveBeenCalledWith(row.saleRef);
  await ReactTestRenderer.act(async () => tree.unmount());
});

test('a reminder reopened thirty seconds before due stays before its deadline', async () => {
  const alerts = Alerts!;
  const device = Http!;
  const clock = jest.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-07T23:59:30Z'));
  jest.mocked(alerts.permission).mockResolvedValueOnce(true);
  jest.mocked(device.loadJournal).mockResolvedValueOnce(JSON.stringify({
    branch: 'branch', owner: 'customer', enabled: true, scheduled: [],
  }));
  const invoice = {
    id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    saleRef: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
    cycle: '2026-11-01', dueAt: '2026-11-08T00:00:00Z',
    state: 'preview', amount: '30', asset: '0x' + '34'.repeat(20),
  };
  const client = {
    profile: { branchRef: 'branch', subjectRef: 'customer' },
    subscriptionOffers: async () => [],
    subscriptions: async () => [],
    invoices: async () => [invoice],
  } as unknown as CommerceClient;
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(
        <SubscriptionScreen client={client} keyFor={async () => 'id'} onInvoice={jest.fn()} />,
      );
    });
    expect(alerts.schedule).toHaveBeenCalledTimes(1);
    const trigger = jest.mocked(alerts.schedule).mock.calls[0][1];
    expect(trigger).toBeGreaterThan(Date.now());
    expect(trigger).toBeLessThan(Date.parse(invoice.dueAt));
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
    clock.mockRestore();
  }
});

test('a future cycle schedules without an invoice and a foreign notification cannot open an owned invoice', async () => {
  const clock = jest.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-01T00:00:00Z'));
  const future = '2026-12-01T00:00:00Z';
  const schedule = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
  jest.mocked(Alerts!.schedule).mockClear().mockResolvedValue(true);
  jest.mocked(Alerts!.permission).mockResolvedValue(true);
  jest.mocked(Http!.loadJournal).mockResolvedValueOnce(JSON.stringify({
    branch: 'branch', owner: 'customer', enabled: true, scheduled: [],
  }));
  const sale = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
  const client = {
    profile: { branchRef: 'branch', subjectRef: 'customer' },
    subscriptionOffers: async () => [],
    subscriptions: async () => [{
      id: schedule, nextAt: future, state: 'active', needsConsent: false,
      amount: '30', asset: '0x' + '34'.repeat(20),
    }],
    invoices: async () => [{
      id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', saleRef: sale, subscriptionRef: schedule,
      dueAt: '2026-10-09T00:00:00Z', state: 'preview', amount: '30', asset: '0x' + '34'.repeat(20),
    }],
  } as unknown as CommerceClient;
  const onInvoice = jest.fn();
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(
        <SubscriptionScreen client={client} keyFor={async () => 'id'} onInvoice={onInvoice}
          alertRef="ffffffff-ffff-4fff-8fff-ffffffffffff" />,
      );
    });
    expect(Alerts!.schedule).toHaveBeenCalledWith(
      schedule, Date.parse(future) + 120_000, Date.parse(future) + 86_400_000,
    );
    expect(tree.root.findAllByType(Text).map(node => node.props.children))
      .toContain('이 계정에서 열 수 없는 주문입니다.');
    expect(tree.root.findAll(node => node.props.accessibilityLabel === '같은 회차 결제 검토')).toHaveLength(0);
    expect(onInvoice).not.toHaveBeenCalled();
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
    clock.mockRestore();
  }
});

test('re-enrollment clears native reminders and local references only on a new scope', async () => {
  const prior = JSON.stringify({ branch: 'old', owner: 'owner', enabled: true, scheduled: [] });
  jest.mocked(Alerts!.clear).mockClear();
  jest.mocked(Http!.saveJournal).mockClear();
  jest.mocked(Http!.loadJournal).mockResolvedValueOnce(prior).mockResolvedValueOnce(prior);
  await isolateSubscriptionReminders('old', 'owner');
  expect(Alerts!.clear).not.toHaveBeenCalled();
  await isolateSubscriptionReminders('other', 'new-owner');
  expect(Alerts!.clear).toHaveBeenCalledTimes(1);
  expect(Http!.saveJournal).toHaveBeenCalledWith('subscription-alerts-v2', '');
});

test('a cleanup reply after the new enrollment has left the screen preserves the previous journal', async () => {
  const previous = JSON.stringify({branch: 'old', owner: 'owner', enabled: true, scheduled: ['old-alert']});
  let active = true;
  let finish!: () => void;
  let started!: () => void;
  const waiting = new Promise<void>(resolve => { started = resolve; });
  jest.mocked(Http!.loadJournal).mockResolvedValueOnce(previous);
  const cleanup = new Promise<void>(resolve => { finish = resolve; });
  jest.mocked(Alerts!.clear).mockImplementationOnce(() => {
    started();
    return cleanup;
  });
  jest.mocked(Http!.saveJournal).mockClear();
  const reviewed = isolateSubscriptionReminders('new', 'owner', () => active);
  await waiting;
  active = false;
  finish();
  await reviewed;
  expect(Http!.saveJournal).not.toHaveBeenCalled();
});

test('an enrolled notification opens the exact server-owned invoice instead of its neighboring cycle', async () => {
  const selected = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
  const openSale = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
  const rows = [
    {id: selected, saleRef: openSale, dueAt: '2026-11-08T00:00:00Z', state: 'preview', amount: '30'},
    {id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', saleRef: 'different-sale', dueAt: '2026-11-15T00:00:00Z', state: 'preview', amount: '30'},
  ];
  const client = {
    profile: {branchRef: 'branch', subjectRef: 'customer'},
    subscriptionOffers: async () => [],
    subscriptions: async () => [],
    invoices: async () => rows,
  } as unknown as CommerceClient;
  const onInvoice = jest.fn();
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(
        <SubscriptionScreen client={client} keyFor={async () => 'id'} onInvoice={onInvoice} alertRef={selected} />,
      );
    });
    const review = tree.root.findAll(node =>
      node.props.accessibilityLabel === '같은 회차 결제 검토' && typeof node.props.onPress === 'function',
    );
    expect(review).toHaveLength(1);
    await ReactTestRenderer.act(async () => review[0].props.onPress());
    expect(onInvoice).toHaveBeenCalledWith(openSale);
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('a delayed revoked-permission check cannot overwrite the new owner journal', async () => {
  let rejectOld!: (allowed: boolean) => void;
  const delayed = new Promise<boolean>(resolve => { rejectOld = resolve; });
  jest.mocked(Alerts!.permission).mockReset().mockReturnValueOnce(delayed).mockResolvedValue(false);
  jest.mocked(Http!.loadJournal).mockReset()
    .mockResolvedValueOnce(JSON.stringify({ branch: 'branch', owner: 'old', enabled: true, scheduled: ['old-ref'] }))
    .mockResolvedValueOnce(JSON.stringify({ branch: 'branch', owner: 'new', enabled: false, scheduled: [] }));
  jest.mocked(Http!.saveJournal).mockClear();
  const client = (owner: string) => ({
    profile: { branchRef: 'branch', subjectRef: owner },
    subscriptionOffers: async () => [], subscriptions: async () => [], invoices: async () => [],
  }) as unknown as CommerceClient;
  const render = (owner: string) => <SubscriptionScreen client={client(owner)} keyFor={async () => 'id'} onInvoice={jest.fn()} />;
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => { tree = ReactTestRenderer.create(render('old')); });
    expect(Alerts!.permission).toHaveBeenCalledTimes(1);
    await ReactTestRenderer.act(async () => tree.update(render('new')));
    await ReactTestRenderer.act(async () => rejectOld(false));
    const saved = jest.mocked(Http!.saveJournal).mock.calls.map(([, journal]) => journal);
    expect(saved.filter(value => value.includes('"owner":"old"'))).toHaveLength(0);
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('an alarm that finishes during a scope change is canceled before the new journal is loaded', async () => {
  let finishOld!: (scheduled: boolean) => void;
  const delaying = new Promise<boolean>(resolve => { finishOld = resolve; });
  const alarm = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';
  jest.mocked(Alerts!.permission).mockResolvedValue(true);
  jest.mocked(Alerts!.schedule).mockReset().mockReturnValueOnce(delaying);
  jest.mocked(Alerts!.remove).mockClear();
  jest.mocked(Http!.loadJournal).mockReset()
    .mockResolvedValueOnce(JSON.stringify({ branch: 'branch', owner: 'old', enabled: true, scheduled: [] }))
    .mockResolvedValueOnce(JSON.stringify({ branch: 'branch', owner: 'new', enabled: false, scheduled: [] }));
  jest.mocked(Http!.saveJournal).mockClear();
  const client = (owner: string) => ({
    profile: { branchRef: 'branch', subjectRef: owner },
    subscriptionOffers: async () => [], subscriptions: async () => [],
    invoices: async () => owner === 'old' ? [{
      id: alarm, saleRef: 'sale', dueAt: new Date(Date.now() + 600_000).toISOString(),
      state: 'preview', amount: '30',
    }] : [],
  }) as unknown as CommerceClient;
  const render = (owner: string) => <SubscriptionScreen client={client(owner)} keyFor={async () => 'id'} onInvoice={jest.fn()} />;
  let tree!: ReactTestRenderer.ReactTestRenderer;
  try {
    await ReactTestRenderer.act(async () => { tree = ReactTestRenderer.create(render('old')); });
    expect(Alerts!.schedule).toHaveBeenCalledTimes(1);
    await ReactTestRenderer.act(async () => tree.update(render('new')));
    await ReactTestRenderer.act(async () => finishOld(true));
    expect(Alerts!.remove).toHaveBeenCalledWith(alarm);
    expect(Http!.saveJournal).not.toHaveBeenCalled();
    expect(jest.mocked(Alerts!.remove).mock.invocationCallOrder[0])
      .toBeLessThan(jest.mocked(Http!.loadJournal).mock.invocationCallOrder[1]);
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});
