import Renderer from 'react-test-renderer';
import {AppState} from 'react-native';
import {TravelPrivate} from '../src/travel/TravelPrivate.tsx';
import {TravelItinerary} from '../src/travel/TravelItinerary.tsx';
import {TravelVisits} from '../src/travel/TravelVisits.tsx';
import {deviceJournal, writeLocalRecordings} from '../src/travel/localRecordings.ts';
import type {Consent} from '../src/travel/privateClient.ts';

const audioA = 'e8e87888-20e2-4237-841c-cc32cc1a3787';
const audioB = '36d6dac5-5ff1-45c6-98b3-dc054addd976';
const plan = '43a668b5-142a-4e04-b178-bd736c30779d';
const receipt = '5a613e0c-85a1-429a-8550-e54a87cafd51';
const mockJournal = new Map<string, string>();
const mockExchange = jest.fn<Promise<string>, [string, string, string]>();
const mockProfile = jest.fn<Promise<string>, [string]>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: (scope: string) => mockProfile(scope),
    exchangeTravel: (method: string, path: string, envelope: string) => mockExchange(method, path, envelope),
    randomBytes: async (size: number) => size === 32
      ? 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA='
      : 'AAAAAAAAAAAAAAAAAAAAAA==',
    discardTravelRecording: async () => {},
    loadJournal: async (name: string) => mockJournal.get(name) ?? '',
    saveJournal: async (name: string, value: string) => { mockJournal.set(name, value); },
  },
}));

function findButton(tree: Renderer.ReactTestRenderer, label: string) {
  const button = tree.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
  expect(button).toBeDefined();
  return button;
}

async function press(tree: Renderer.ReactTestRenderer, label: string) {
  const button = findButton(tree, label);
  expect(button.props.disabled).toBeFalsy();
  await Renderer.act(async () => { button.props.onPress(); });
}

const expiry = new Date(Date.now() + 36 * 3600_000).toISOString();

beforeEach(() => {
  mockExchange.mockReset();
  mockProfile.mockReset();
  mockProfile.mockResolvedValue(JSON.stringify({scope: {role: 'private-user', expiresAt: Date.now() / 1000 + 300},
    policies: [{purpose: 'voice', version: 1, recipient: 'p09-sync', maxLifetimeSeconds: 1200, targetRequired: false}]}));
  mockJournal.clear();
});

test('resumes selected-record erasure and acknowledges only the original local audio copy', async () => {
  mockJournal.set(deviceJournal(audioA), 'fixture audio A');
  mockJournal.set(deviceJournal(audioB), 'fixture audio B');
  await writeLocalRecordings([{id: audioA, capturedAt: '2026-11-20T00:00:00Z'},
    {id: audioB, capturedAt: '2026-11-20T01:00:00Z'}]);
  const requestRef = '00000000-0000-4000-8000-000000000000';
  const installationProof = '00'.repeat(32);
  let requested = false;
  mockExchange.mockImplementation(async (method, path) => {
    if (path === '/consents') return JSON.stringify({consents: []});
    if (path === '/records') return JSON.stringify({records: (requested ? [audioB] : [audioA, audioB])
      .map(id => ({id, purpose: 'voice', consentRevision: 1, deleteAfter: expiry}))});
    if (path === '/erasure' && method === 'GET') return JSON.stringify({receipts: requested ?
      [{request: requestRef, requestedAt: expiry}] : []});
    if (path === '/erasure') requested = true;
    return JSON.stringify({request: requestRef, state: 'access-fenced', backupExpiry: expiry,
      localDeviceAck: path !== '/erasure' && method === 'POST'});
  });
  let tree: Renderer.ReactTestRenderer | undefined;
  try {
    await Renderer.act(async () => { tree = Renderer.create(<TravelPrivate back={() => {}} places={[]} />); });
    await press(tree!, '이 기록 삭제 검토');
    await press(tree!, '이 기록과 파생물 삭제 요청');
    expect(findButton(tree!, '이 기록 삭제 검토').props.disabled).toBe(true);
    const begin = mockExchange.mock.calls.find(call => call[0] === 'POST' && call[1] === '/erasure');
    expect(JSON.parse(begin![2])).toEqual({object: audioA, requestKey: requestRef, installationProof});
    await Renderer.act(async () => { tree!.unmount(); });
    await Renderer.act(async () => { tree = Renderer.create(<TravelPrivate back={() => {}} places={[]} />); });
    await press(tree!, '선택한 기록의 삭제 상태 열기');
    await press(tree!, '이 기기의 복사본 삭제와 확인');
    expect(mockJournal.get(deviceJournal(audioA))).toBe('');
    expect(mockJournal.get(deviceJournal(audioB))).toBe('fixture audio B');
    expect(mockExchange).toHaveBeenCalledWith('POST', `/erasure/${requestRef}`, JSON.stringify({localDeviceErased: true, installationProof}));
    await press(tree!, '완료한 삭제 상태 닫기');
    await press(tree!, '삭제 영수증 보기');
    expect(mockExchange).toHaveBeenCalledWith('GET', `/erasure/${requestRef}`, '');
    expect(mockJournal.get(deviceJournal(audioB))).toBe('fixture audio B');
  } finally {
    if (tree) await Renderer.act(async () => { tree!.unmount(); });
  }
});

test.each(['01:00', '03:00'])('at %s KST a stale plan saves only reselected candidates after explicit review', async (seoulTime) => {
  const now = Date.parse(`2026-11-20T${seoulTime}:00+09:00`);
  const clock = jest.spyOn(Date, 'now').mockReturnValue(now);
  const expiry = '2026-11-21T06:00:00.000Z'; // 15:00 Seoul, after the selected next-day window.
  mockProfile.mockResolvedValue(JSON.stringify({scope: {role: 'private-user', expiresAt: now / 1000 + 300},
    policies: [{purpose: 'recommendation', version: 1, recipient: 'p09', maxLifetimeSeconds: 48 * 3600, targetRequired: false}]}));
  const park = {id: 'park', name: '중앙 공원', address: '서울', source: '승인 목록', updatedAt: expiry, region: 'seoul',
    opening: [{weekday: 1, timezone: 'Asia/Seoul', opens: '09:00', closes: '17:00'}]};
  const consent: Consent = {purpose: 'recommendation', revision: 1, policyVersion: 1,
    recipient: 'p09', accepted: true, expiresAt: expiry, receipt};
  const original = {place: 'park', start: '2026-11-21T01:00:00Z', end: '2026-11-21T02:00:00Z'};
  const latest = {place: 'library', start: '2026-11-21T02:00:00Z', end: '2026-11-21T03:00:00Z'};
  const selected = {place: 'museum', start: '2026-11-21T03:00:00Z', end: '2026-11-21T04:00:00Z'};
  const discarded = {place: 'garden', start: '2026-11-21T04:00:00Z', end: '2026-11-21T05:00:00Z'};
  const saved = {id: plan, consentRevision: 1, catalogVersion: 1, expiresAt: expiry, budgetLimit: '17000',
    budgetUnit: 'KRW', stops: [latest]};
  const candidate = {...saved, budgetLimit: '15000', stops: [original, selected, discarded]};
  let remote = {revision: 2, plan: saved};
  mockExchange.mockImplementation(async (_, path, envelope) => {
    if (path === '/records') {
      expect(JSON.parse(envelope)).toEqual({projection: 'saved-itineraries'});
      return JSON.stringify({itineraries: [{id: plan, revision: remote.revision, place: 'library', name: '도서관', start: latest.start}]});
    }
    if (path === '/itineraries/generate') {
      expect(JSON.parse(envelope)).toMatchObject({
        start: '2026-11-21T01:00:00.000Z', end: '2026-11-21T05:00:00.000Z', expiresAt: expiry,
      });
      return JSON.stringify({preview: candidate, confirmation: 'old-confirmation', provider: 'fixture'});
    }
    if (path === '/itineraries/preview') {
      const body = JSON.parse(envelope);
      expect(body).toMatchObject({expectedRevision: 2, budgetLimit: '17000', stops: [latest, selected]});
      return JSON.stringify({preview: {...saved, stops: body.stops}, confirmation: 'new-confirmation'});
    }
    if (path === '/itineraries/apply') {
      const body = JSON.parse(envelope);
      if (body.confirmedPreview === 'old-confirmation') return '{"error":"stale_or_conflicting_intent"}';
      expect(body).toMatchObject({expectedRevision: 2, confirmedPreview: 'new-confirmation', stops: [latest, selected]});
      remote = {revision: 3, plan: {...saved, stops: body.stops}};
      return '{"revision":3}';
    }
    return JSON.stringify(remote);
  });
  const screen = () => <TravelItinerary place={park} places={[park, {...park, id: 'library', name: '도서관'},
    {...park, id: 'museum', name: '박물관'}, {...park, id: 'garden', name: '정원'}]} catalogVersion={1} consent={consent} />;
  const writes = () => mockExchange.mock.calls.filter(call => call[1] === '/itineraries/apply');
  let tree: Renderer.ReactTestRenderer | undefined;
  try {
    await Renderer.act(async () => { tree = Renderer.create(screen()); });
    await press(tree!, '날짜와 선호로 코스 추천');
    await press(tree!, '검토한 일정 적용');
    expect(writes()).toHaveLength(1);
    await press(tree!, '변경된 최신 일정 열기');
    expect(mockExchange).toHaveBeenCalledWith('GET', `/itineraries/${plan}`, '');
    expect(writes()).toHaveLength(1);
    expect(findButton(tree!, '선택한 후보 검토').props.disabled).toBe(true);
    await press(tree!, '비교 닫기');
    expect(writes()).toHaveLength(1);
    await press(tree!, '날짜와 선호로 코스 추천');
    await press(tree!, '검토한 일정 적용');
    await press(tree!, '변경된 최신 일정 열기');
    await press(tree!, '후보 2 박물관 선택');
    await press(tree!, '선택한 후보 검토');
    expect(writes()).toHaveLength(2);
    const confirm = findButton(tree!, '검토한 일정 적용');
    await Renderer.act(async () => { confirm.props.onPress(); confirm.props.onPress(); });
    expect(writes()).toHaveLength(3);
    expect(remote.revision).toBe(3);
    await Renderer.act(async () => { tree!.unmount(); });
    await Renderer.act(async () => { tree = Renderer.create(screen()); });
    expect(writes()).toHaveLength(3);
    expect(tree!.root.findAll(node => node.props.accessibilityLabel === '검토한 일정 적용')).toHaveLength(0);
  } finally {
    try {
      if (tree) await Renderer.act(async () => { tree!.unmount(); });
    } finally {
      clock.mockRestore();
    }
  }
});

test('keeps the current visit cursor on refresh, foreground, and incidental re-renders', async () => {
  const next = 'c'.repeat(64);
  let foreground: (() => void) | undefined;
  const listener = jest.spyOn(AppState, 'addEventListener').mockImplementation((_name, callback) => {
    foreground = () => callback('active');
    return {remove: jest.fn()};
  });
  let tree!: Renderer.ReactTestRenderer;
  try {
    mockExchange.mockImplementation(async (_method, path, envelope) => {
      if (path === '/consents') return JSON.stringify({consents: []});
      const {cursor} = JSON.parse(envelope);
      return JSON.stringify({options: [], challengeCount: 0, next: cursor ? undefined : next});
    });
    const calls = () => mockExchange.mock.calls.filter(([, path]) => path === '/visits').map(([, , body]) => JSON.parse(body).cursor);
    await Renderer.act(async () => {
      tree = Renderer.create(<TravelVisits screen="challenge" places={[]} policies={[]} />);
    });
    expect(calls()).toEqual([undefined]);
    await press(tree, '지난 이용 내역 더 보기');
    await Renderer.act(async () => {
      tree.update(<TravelVisits screen="challenge" places={[]} policies={[]} />);
    });
    expect(calls()).toEqual([undefined, next]);
    await press(tree, '이용과 혜택 상태 다시 조회');
    await Renderer.act(async () => foreground?.());
    expect(calls()).toEqual([undefined, next, next, next]);
    await press(tree, '이전 내역 페이지');
    expect(calls()).toEqual([undefined, next, next, next, undefined]);
  } finally {
    if (tree) await Renderer.act(async () => tree.unmount());
    listener.mockRestore();
  }
});

test('a traveller without a private profile can decline and return without a write', async () => {
  const listener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  const back = jest.fn();
  mockProfile.mockResolvedValue('');
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {tree = Renderer.create(<TravelPrivate back={back} places={[]} />);});
  try {
    await press(tree, '동의하지 않음');
    expect(back).toHaveBeenCalledTimes(1);
    expect(mockExchange).not.toHaveBeenCalled();
    expect(mockJournal.size).toBe(0);
  } finally {
    await Renderer.act(async () => {tree.unmount();});
    listener.mockRestore();
  }
});
