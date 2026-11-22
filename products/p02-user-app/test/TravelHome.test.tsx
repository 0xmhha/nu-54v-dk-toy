import {AppState, Image, TextInput, View, type AppStateStatus} from 'react-native';
import Renderer from 'react-test-renderer';
import {TravelHome} from '../src/travel/TravelHome.tsx';

const mockOrigin = 'https://places.example';
const mockBranch = 'd6d10219-1c27-492c-9bdf-ec14d7d092bd';
const mockOriginalDeposit = 'c51da18b-566f-437d-9e6e-1471633a1f58';
const mockAsset = '0x' + 'a'.repeat(40);
const mockCalls: string[] = [];
let mockHasEnrollment = false;
let mockBookingMode = false;
let mockBookingIdentity = mockBranch;
let mockRegisteredBranch = mockBranch;
let mockPickupReply: Promise<string> | undefined;
let mockBookingDelay: Promise<void> | undefined;
let mockCancellationPreview: object | undefined;
let mockCancellationDelay: Promise<void> | undefined;
let mockBookingCutoff: string | undefined;
let mockDepositCancellation: 'held'|'cancelling'|'cancelled'|undefined;
const mockBookingExpiry = Math.floor(Date.now() / 1000) + 300;
const mockReadProfile = jest.fn();
const mockPrivateTravel = jest.fn<Promise<string>, [string, string, string]>();
let mockPrivateEnrollment = '';
const mockSaveTravelJournal = jest.fn();
jest.mock('../src/travel/publicDeployment.ts', () => ({approvedTravelOrigin: 'https://places.example'}));

jest.mock('../src/specs/NativeRenterBle.ts', () => ({
  __esModule: true,
  default: {loadDevices: async () => ''},
}));

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadJournal: async () => '',
    saveJournal: async () => {mockSaveTravelJournal();},
    discardTravelRecording: async () => {},
    exchangeTravel: (method: string, path: string, body: string) => mockPrivateTravel(method, path, body),
    loadProfile: async (role: string) => {
      mockReadProfile();
      if (role === 'travel') return mockPrivateEnrollment;
      if (!mockHasEnrollment) return '';
      return JSON.stringify({
      scope: {role: 'customer', branchRef: mockRegisteredBranch, subjectRef: mockBookingIdentity, tenantRef: mockBranch,
        expiresAt: mockBookingExpiry},
      pin: 'a'.repeat(64),
      payment: {chain: 8283, contract: '0x' + 'b'.repeat(40), beneficiary: '0x' + 'c'.repeat(40),
        signer: '0x' + 'd'.repeat(40), asset: mockAsset, merchantId: 'e'.repeat(64), keyVersion: 1,
        beneficiaryVersion: 1, maxBaseUnits: '30000000'},
    });},
    exchange: async (slot: string, command: string) => {
      const {travel} = JSON.parse(command);
      mockCalls.push(`${slot}/${travel.operation}/${travel.branch}`);
      if (travel.operation === 'pickup-orders' && mockPickupReply) return mockPickupReply;
      if (mockBookingMode) {
        if (travel.operation === 'my-bookings') {
          if (mockBookingDelay) await mockBookingDelay;
          return JSON.stringify({bookings: [{id: mockBranch, partySize: 1, state: mockDepositCancellation ?? (mockCancellationPreview ? 'pending_payment' : 'cancelling'),
            termsVersion: 1, expiresAt: '2026-12-01T12:00:00Z', cancellation: '예약 조건', cancellationDeadline: mockBookingCutoff,
            asset: mockAsset, deposit: '10000000', balance: '20000000'}]});
        }
        if (travel.operation === 'cancel-preview' && mockCancellationPreview) {
          if (mockCancellationDelay) await mockCancellationDelay;
          return JSON.stringify({cancellationPreview: mockCancellationPreview});
        }
        if (mockDepositCancellation && travel.operation === 'cancel') {
          expect(travel.booking).toBe(mockBranch);
          mockDepositCancellation = 'cancelling';
          return JSON.stringify({bookingRef:mockBranch});
        }
        if (travel.operation === 'legs') {
          if (mockDepositCancellation) return JSON.stringify({legs: [{
            kind:'deposit', saleRef:mockOriginalDeposit, amount:'10000000', state:'paid', receiptRef:'original-receipt',
            refunded:mockDepositCancellation==='cancelled'?'10000000':'0', refundExpected:'10000000',
            refundState:mockDepositCancellation==='cancelled'?'settled':'waiting-refund',
          }]});
          return JSON.stringify({legs: [
            {kind: 'deposit', saleRef: 'deposit', amount: '10000000', state: 'paid', receiptRef: 'dep-receipt', refunded: '10000000', refundExpected: '10000000', refundState: 'settled'},
            {kind: 'balance', saleRef: 'balance', amount: '20000000', state: 'paid', receiptRef: 'balance-receipt', refunded: '0', refundExpected: '20000000', refundState: 'waiting-refund'},
          ]});
        }
        throw new Error("The reopened booking must remain read-only");
      }
      return JSON.stringify({slots: [{
        id: mockBranch, termsVersion: 1, remaining: 3, startsAt: '2026-12-01T12:00:00Z',
        cancellation: '방문 전 예약금 일부 환불', asset: mockAsset, deposit: '10000000', balance: '20000000',
      }]});
    },
  },
}));

test('shows an owner-scoped estimate without cancelling and discards a stale response', async () => {
  mockCalls.length = 0;
  mockHasEnrollment = mockBookingMode = true;
  mockCancellationPreview = {canCancel: true, cancellationDeadline: new Date(Date.now() + 120_000).toISOString(), legs: [
    {kind: 'deposit', saleRef: mockBranch, paymentState: 'paid', refundExpected: '3000000'},
    {kind: 'balance', saleRef: '9ac32a03-f6d0-40f3-aad7-c898de201e06', paymentState: 'issued'},
  ]};
  let app: Renderer.ReactTestRenderer | undefined;
  let release: (() => void) | undefined;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={async () => false}
        bookingScope={{branch: mockBranch, customer: mockBranch, tenant: mockBranch}} />);
    });
    const buttons = (label: string) => app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function');
    const press = async (label: string) => {
      await Renderer.act(async () => { buttons(label)[0].props.onPress(); });
    };
    await press('예약 취소 검토');
    expect(JSON.stringify(app!.toJSON())).toContain('예상 환불 3.00 TestUSDC');
    expect(JSON.stringify(app!.toJSON())).toContain('지급이 확정되지 않아 예상 환불액을 표시할 수 없습니다.');
    expect(buttons('예약 취소 요청 확인')).not.toHaveLength(0);
    expect(mockCalls.filter(value => value.includes('/cancel/'))).toHaveLength(0);
    await press('예약 유지');
    const timelyPreview = mockCancellationPreview;
    mockCancellationPreview = {...timelyPreview, cancellationDeadline: new Date(Date.now() - 1000).toISOString()};
    await press('예약 취소 검토');
    expect(buttons('예약 취소 요청 확인')).toHaveLength(0);
    expect(JSON.stringify(app!.toJSON())).toContain('이 예약은 현재 취소할 수 없습니다.');
    await press('예약 유지');
    mockCancellationPreview = timelyPreview;
    mockCancellationDelay = new Promise<void>(resolve => { release = resolve; });
    await press('예약 취소 검토');
    mockBookingIdentity = '2b802d84-2f99-4201-b090-1eaef38129c4';
    await Renderer.act(async () => { release?.(); });
    expect(buttons('예약 취소 요청 확인')).toHaveLength(0);
    expect(JSON.stringify(app!.toJSON())).not.toContain('예상 환불 3.00 TestUSDC');
    expect(mockCalls.filter(value => value.includes('/cancel/'))).toHaveLength(0);
  } finally {
    release?.();
    await Renderer.act(async () => { app?.unmount(); });
    mockHasEnrollment = mockBookingMode = false;
    mockBookingIdentity = mockBranch;
    mockCancellationPreview = mockCancellationDelay = undefined;
    mockReadProfile.mockClear();
    mockCalls.length = 0;
  }
});

test('T07 confirms cancellation explicitly and reopens the same deposit after refund',async()=>{
  mockCalls.length=0;
  mockHasEnrollment=mockBookingMode=true;
  mockDepositCancellation='held';
  mockBookingCutoff=new Date(Date.now()+120_000).toISOString();
  mockCancellationPreview={canCancel:true,cancellationDeadline:new Date(Date.now()+180_000).toISOString(),
    legs:[{kind:'deposit',saleRef:mockOriginalDeposit,paymentState:'paid',refundExpected:'10000000'}]};
  const screen=()=> <TravelHome back={()=>{}} permission={async()=>false}
    bookingScope={{branch:mockBranch,customer:mockBranch,tenant:mockBranch}}/>;
  let app!:Renderer.ReactTestRenderer;
  const text=()=>JSON.stringify(app.toJSON());
  const press=async(label:string)=>{
    const button=app.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    expect(button.props.disabled).not.toBe(true);
    await Renderer.act(async()=>button.props.onPress());
  };
  try{
    await Renderer.act(async()=>{app=Renderer.create(screen());});
    await press('예약 취소 검토');
    expect(text()).toContain('이 예약의 예상 환불액을 확인할 수 없습니다.');
    expect(mockCalls.filter(value=>value.includes('/cancel/'))).toHaveLength(0);
    mockCancellationPreview={...mockCancellationPreview,cancellationDeadline:mockBookingCutoff};
    await press('예약 취소 검토');
    expect(text()).toContain('예상 환불 10.00 TestUSDC');
    expect(mockCalls.filter(value=>value.includes('/cancel/'))).toHaveLength(0);
    await press('예약 취소 요청 확인');
    expect(mockCalls.filter(value=>value.includes('/cancel/'))).toHaveLength(1);
    expect(text()).toContain('예약금 환불 확인 중');
    expect(text()).toContain(mockOriginalDeposit);
    expect(text()).toContain('original-receipt');
    expect(text()).not.toContain('확정 환불 10.00 TestUSDC');
    await Renderer.act(async()=>app.unmount());
    mockDepositCancellation='cancelled';
    await Renderer.act(async()=>{app=Renderer.create(screen());});
    await press('예약 지급 및 영수증 조회');
    expect(text()).toContain('확정 환불 10.00 TestUSDC');
    expect(text()).toContain(mockOriginalDeposit);
    expect(text()).toContain('original-receipt');
    expect(mockCalls.filter(value=>value.includes('/cancel/'))).toHaveLength(1);
  }finally{
    await Renderer.act(async()=>app?.unmount());
    mockHasEnrollment=mockBookingMode=false;
    mockCancellationPreview=mockDepositCancellation=mockBookingCutoff=undefined;
    mockReadProfile.mockClear();
    mockCalls.length=0;
  }
});

test('T07 removes the active cancellation action exactly at the recorded cutoff',async()=>{
  jest.useFakeTimers();
  mockCalls.length=0;
  mockHasEnrollment=mockBookingMode=true;
  mockDepositCancellation='held';
  mockBookingCutoff=new Date(Date.now()+5000).toISOString();
  mockCancellationPreview={canCancel:true,cancellationDeadline:mockBookingCutoff,
    legs:[{kind:'deposit',saleRef:mockOriginalDeposit,paymentState:'paid',refundExpected:'10000000'}]};
  let app!:Renderer.ReactTestRenderer;
  const buttons=(label:string)=>app.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function');
  try{
    await Renderer.act(async()=>{app=Renderer.create(<TravelHome back={()=>{}} permission={async()=>false}
      bookingScope={{branch:mockBranch,customer:mockBranch,tenant:mockBranch}}/>);});
    await Renderer.act(async()=>buttons('예약 취소 검토')[0].props.onPress());
    await Renderer.act(async()=>jest.advanceTimersByTime(4999));
    const confirm=buttons('예약 취소 요청 확인')[0].props.onPress;
    expect(JSON.stringify(app.toJSON())).toContain('예상 환불 10.00 TestUSDC');
    await Renderer.act(async()=>jest.advanceTimersByTime(1));
    expect(buttons('예약 취소 요청 확인')).toHaveLength(0);
    expect(JSON.stringify(app.toJSON())).not.toContain('예상 환불 10.00 TestUSDC');
    await Renderer.act(async()=>confirm());
    expect(mockCalls.filter(value=>value.includes('/cancel/'))).toHaveLength(0);
  }finally{
    await Renderer.act(async()=>app?.unmount());
    mockHasEnrollment=mockBookingMode=false;
    mockCancellationPreview=mockDepositCancellation=mockBookingCutoff=undefined;
    mockReadProfile.mockClear();
    mockCalls.length=0;
    jest.useRealTimers();
  }
});

test('accesses published Korean discovery without permission and shows enrolled human totals', async () => {
  const assetUnavailable = jest.fn(async () => false);
  const place = {region: 'seoul', address: '서울', source: '시험 자료', updatedAt: '2026-11-20T00:00:00Z', latitude: 37.53, longitude: 126.91};
  const requests: string[] = [];
  const source = jest.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith('/v1/region-options')) {
      return {ok: true, url, json: async () => [{code: 'seoul', name: '서울'}]} as Response;
    }
    if (url.endsWith('/v1/map')) {
      return {ok: true, url, json: async () => ({minZoom: 7, maxZoom: 9, attribution: '승인된 기준지도'})} as Response;
    }
    return {ok: true, url, json: async () => ({environment: 'synthetic', places: [
      {...place, id: 'park', name: '서울공원', category: 'attraction'},
      {...place, id: 'dining', name: '한식당', category: 'restaurant', branchRef: mockBranch, capabilities: ['booking']},
    ]})} as Response;
  });
  let app: Renderer.ReactTestRenderer | undefined;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={assetUnavailable} />);
    });
    const press = async (label: string) => {
      const found = app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
      expect(found).toBeDefined();
      await Renderer.act(async () => {found!.props.onPress();});
    };
    const has = (label: string) => app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function').length > 0;
    await Renderer.act(async () => {
      app!.root.findAllByType(TextInput).find(input => input.props.accessibilityLabel === '지역 이름')!.props.onChangeText('서울');
    });
    await press('내 여행');
    expect(JSON.stringify(app!.toJSON())).toContain('나의 여행');
    for (const action of ['예약', '후기', '스탬프', '기록과 동의', 'AI 코스', '챌린지', '탐색']) expect(has(action)).toBe(true);
    expect(mockReadProfile).not.toHaveBeenCalled();
    expect(mockCalls).toEqual([]);
    await press('탐색');
    await press('서울');
    await press('서울공원');
    expect(has('예약 가능한 시간')).toBe(false);
    await press('맛집');
    expect(has('서울공원')).toBe(false);
    await press('한식당');
    expect(mockReadProfile).not.toHaveBeenCalled();
    await press('예약 가능한 시간');
    expect(mockReadProfile).toHaveBeenCalledTimes(1);
    expect(mockCalls).toEqual([]);
    mockHasEnrollment = true;
    await press('예약 가능한 시간');
    await Renderer.act(async () => {
      app!.root.findAllByType(TextInput).find(input => input.props.accessibilityLabel === '인원')!.props.onChangeText('2');
    });
    const contents = JSON.stringify(app!.toJSON());
    expect(contents).toContain('10.00 TestUSDC');
    expect(contents).toContain('20.00 TestUSDC');
    expect(contents).toContain('60.00 TestUSDC');
    await press('지도');
    expect(JSON.stringify(app!.toJSON())).toContain('승인된 기준지도');
    await Renderer.act(async () => {
      app!.root.findAllByType(View).find(view => view.props.accessibilityLabel === '공개 지역 지도')!
        .props.onLayout({nativeEvent: {layout: {width: 240, height: 230}}});
    });
    const tiles = app!.root.findAllByType(Image);
    expect(tiles.some(tile => tile.props.source.uri === `${mockOrigin}/v1/map/9/436/198.png`)).toBe(true);
    await press('내 여행');
    await press('탐색');
    expect(JSON.stringify(app!.toJSON())).toContain('승인된 기준지도');
    await Renderer.act(async () => {
      app!.root.findAllByType(View).find(view => view.props.accessibilityLabel === '공개 지역 지도')!
        .props.onLayout({nativeEvent: {layout: {width: 240, height: 230}}});
    });
    await Renderer.act(async () => {app!.root.findAllByType(Image)[0].props.onError();});
    await press('목록 보기');
    expect(has('한식당')).toBe(true);
    expect(assetUnavailable).not.toHaveBeenCalled();
    expect(requests).toEqual([`${mockOrigin}/v1/region-options`, `${mockOrigin}/v1/places?region=seoul`, `${mockOrigin}/v1/map`]);
    expect(mockCalls).toEqual([`customer/options/${mockBranch}`]);
  } finally {
    source.mockRestore();
    if (app) await Renderer.act(async () => app?.unmount());
  }
});


test.each(['customer', 'branch', 'background'] as const)('T06 drops delayed pickup history after %s changes', async (change) => {
  mockHasEnrollment = true;
  let release: (value: string) => void = () => {};
  let notify: ((state: AppStateStatus) => void) | undefined;
  const initialStateListener = AppState.addEventListener;
  const initialStateImplementation = jest.isMockFunction(initialStateListener) ? initialStateListener.getMockImplementation() : undefined;
  const state = jest.spyOn(AppState, 'addEventListener').mockImplementation((_, listener) => {
    notify = listener;
    return {remove() {}};
  });
  const original = JSON.stringify({pickups: [{saleRef: mockOriginalDeposit,
    startsAt:'2026-12-01T12:00:00Z', timezone:'Asia/Seoul', amount:'4500000', asset:mockAsset, state:'waiting-payment'}]});
  mockPickupReply = Promise.resolve(original);
  const publicSource = jest.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input);
    return {ok: true, url, json: async () => url.endsWith('/v1/region-options') ? [{code: 'seoul', name: '서울'}] : {
      environment: 'synthetic', places: [{id: 'dining', category: 'restaurant', name: '한식당', address: '서울',
        source: '공개 자료', updatedAt: '2026-11-20T00:00:00Z', region: 'seoul', branchRef: mockBranch, capabilities: ['pickup']}],
    }} as Response;
  });
  let app: Renderer.ReactTestRenderer | undefined;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={async () => false} />);
    });
    const press = async (label: string) => {
      const control = app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
      await Renderer.act(async () => {control.props.onPress();});
    };
    await press('서울');
    await press('한식당');
    await press('내 픽업 주문');
    expect(JSON.stringify(app!.toJSON())).toContain('지급 확인 중');
    mockPickupReply = new Promise(resolve => {release = resolve;});
    await press('내 픽업 주문');
    expect(JSON.stringify(app!.toJSON())).not.toContain('지급 확인 중');
    if (change === 'customer') mockBookingIdentity = mockOriginalDeposit;
    if (change === 'branch') mockRegisteredBranch = mockOriginalDeposit;
    if (change === 'background') await Renderer.act(async () => {notify?.('background');});
    await Renderer.act(async () => {release(original);});
    expect(JSON.stringify(app!.toJSON())).not.toContain('지급 확인 중');
    if (change !== 'background') expect(JSON.stringify(app!.toJSON())).toContain('등록 지점의 픽업 주문을 확인하지 못했습니다.');
  } finally {
    mockBookingIdentity = mockBranch;
    mockRegisteredBranch = mockBranch;
    if (initialStateImplementation) state.mockImplementation(initialStateImplementation);
    else state.mockRestore();
    mockPickupReply = undefined;
    mockHasEnrollment = false;
    publicSource.mockRestore();
    if (app) await Renderer.act(async () => app?.unmount());
  }
});

test('T10 separates explicit private reads from consent-free exploration and never mutates a registered travel profile', async () => {
  const profileUntil = Math.floor(Date.now() / 1000) + 1800;
  mockPrivateEnrollment = JSON.stringify({scope: {role: 'private-user', expiresAt: profileUntil},
    policies: [{purpose: 'recommendation', version: 1, recipient: 'p09', maxLifetimeSeconds: 1200, targetRequired: false}]});
  mockReadProfile.mockClear();
  mockPrivateTravel.mockReset();
  mockSaveTravelJournal.mockClear();
  mockPrivateTravel.mockImplementation(async (_method, path) => {
    switch (path) {
      case '/consents': return JSON.stringify({consents: []});
      case '/records': return JSON.stringify({records: []});
      case '/erasure': return JSON.stringify({receipts: []});
      case '/visits': return JSON.stringify({options: [], challengeCount: 0});
      default: throw new Error('Unexpected travel request');
    }
  });
  const permission = jest.fn(async () => false);
  const source = jest.spyOn(globalThis, 'fetch').mockImplementation(async input =>
    ({ok: true, url: String(input), json: async () => [{code: 'seoul', name: '서울'}]}) as Response);
  let app: Renderer.ReactTestRenderer | undefined;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={permission} />);
    });
    const press = async (label: string) => {
      const control = app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
      expect(control).toBeDefined();
      await Renderer.act(async () => {control.props.onPress();});
    };
    const content = () => JSON.stringify(app!.toJSON());
    await press('내 여행');
    await press('탐색');
    await press('내 여행');
    expect(mockReadProfile).not.toHaveBeenCalled();
    expect(mockPrivateTravel).not.toHaveBeenCalled();
    for (const [route, detail] of [['후기', '후기와 이용 자격'], ['스탬프', '스탬프와 혜택'], ['기록과 동의', '음성 기록'],
      ['AI 코스', '내 일정과 추천'], ['챌린지', '따라하기 챌린지']]) {
      await press(route);
      expect(content()).toContain(detail);
      await press('내 여행으로 돌아가기');
      expect(content()).toContain('나의 여행');
    }
    expect(mockPrivateTravel.mock.calls.filter(([method]) => method !== 'GET')).toEqual(
      expect.arrayContaining([['POST', '/visits', JSON.stringify({projection: 'options', includeBenefits: true})]]),
    );
    for (const [method, path, payload] of mockPrivateTravel.mock.calls) {
      if (method === 'POST') {
        expect(path).toBe('/visits');
        expect(JSON.parse(payload).projection).toBe('options');
      }
    }
    expect(mockSaveTravelJournal).not.toHaveBeenCalled();
    expect(permission).not.toHaveBeenCalled();
    expect(source).toHaveBeenCalledTimes(1);
  } finally {
    mockPrivateEnrollment = '';
    mockPrivateTravel.mockReset();
    source.mockRestore();
    if (app) await Renderer.act(async () => app?.unmount());
  }
});

test('reopens the existing paid booking and displays its partial refund without a new purchase', async () => {
  mockHasEnrollment = true;
  mockBookingMode = true;
  mockCalls.length = 0;
  const publicAPI = jest.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input);
    return {ok: true, url, json: async () => url.endsWith('/v1/region-options')
      ? [{code: 'seoul', name: '서울'}]
      : {environment: 'synthetic', places: [{id: 'dining', region:'seoul', name: '한식당', address: '서울', source: '승인된 시험 목록',
          updatedAt: '2026-11-20T00:00:00Z', category: 'restaurant', branchRef: mockBranch, capabilities: ['booking']}]}} as Response;
  });
  let app: Renderer.ReactTestRenderer | undefined;
  try {
    for (let visit = 0; visit < 2; visit++) {
      await Renderer.act(async () => {app = Renderer.create(<TravelHome back={() => {}} permission={async () => false} />);});
      const press = async (label: string) => {
        const found = app!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
        expect(found).toBeDefined();
        await Renderer.act(async () => {found!.props.onPress();});
      };
      await press('서울');
      await press('한식당');
      await press('내 예약 다시 열기');
      await press('예약 지급 및 영수증 조회');
      const contents = JSON.stringify(app!.toJSON());
      expect(contents).toContain('총액 30.00 TestUSDC');
      expect(contents).toContain('부분 환불');
      expect(contents).toContain('확정 환불 10.00 TestUSDC');
      expect(contents).toContain('잔금 환불 확인 중');
      expect(contents).toContain('dep-receipt');
      expect(contents).toContain('balance-receipt');
      expect(contents).not.toContain('잔금 결제 검토');
      expect(contents).not.toContain('예약금 결제 검토');
      await Renderer.act(async () => app?.unmount());
      app = undefined;
    }
    expect(mockCalls.map(call => call.split('/')[1])).toEqual(['my-bookings', 'legs', 'my-bookings', 'legs']);
  } finally {
    publicAPI.mockRestore();
    mockBookingMode = false;
    if (app) await Renderer.act(async () => app?.unmount());
  }
});

test('T01 combines draft filters and preserves the applied page on failed or older public refresh', async () => {
  const published = {
    region: 'seoul', address: '서울', source: '서명 공개 카탈로그', updatedAt: '2026-11-20T00:00:00Z',
    latitude: 37.53, longitude: 126.91,
  };
  const open = [{weekday: 6, timezone: 'Asia/Seoul', opens: '11:00', closes: '13:00'}];
  const category = 'restaurant';
  const eligible = Array.from({length: 26}, (_, i) => ({
    ...published, category, name: `가게${i + 1}`, id: `cafe-${i + 1}`,
    opening: open, branchRef: mockBranch, capabilities: ['booking'],
  }));
  const places = [
    ...eligible,
    {...published, category, id: 'missing', name: '시간 미게시', branchRef: mockBranch, capabilities: ['booking']},
    {...published, category, id: 'closed', name: '시간 외', opening: [{...open[0], opens:'07:00', closes:'09:00'}],
      branchRef: mockBranch, capabilities: ['booking']},
    {...published, category, id: 'walkin', name: '예약 기능 없음', opening: open},
    {...published, category:'attraction', id:'park', name:'서울 공원', opening: open},
  ];
  const permission = jest.fn(async () => false);
  const clock = jest.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-21T03:00:00Z'));
  let result: 'current' | 'stale' | 'offline' = 'current';
  const source = jest.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input);
    if (url.endsWith('/v1/region-options')) return {ok: true, url, json: async () => [{code:'seoul', name:'서울'}]} as Response;
    if (url.endsWith('/v1/map')) return {ok:true,url,json:async () => ({minZoom:7,maxZoom:9,attribution:'공개 지도'})} as Response;
    if (result === 'offline') throw new Error('offline');
    return {ok: true, url, json: async () => ({environment:'synthetic', version:result === 'stale' ? 1 : 2,
      places: result === 'stale' ? [] : places})} as Response;
  });
  mockReadProfile.mockClear();
  let app!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {app = Renderer.create(<TravelHome back={() => {}} permission={permission} />);});
    const buttons = (label: string) => app.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function');
    const press = async (label: string) => {
      const button = buttons(label)[0];
      expect(button).toBeDefined();
      await Renderer.act(async () => {button.props.onPress();});
    };
    const contents = () => JSON.stringify(app.toJSON());
    await press('서울');
    await press('필터');
    await press('맛집 유형');
    await press('영업 중');
    await press('예약 가능');
    expect(contents()).not.toContain('적용한 필터');
    await press('취소');
    expect(contents()).not.toContain('적용한 필터');
    await press('필터');
    await press('맛집 유형');
    await press('영업 중');
    await press('예약 가능');
    await press('필터 적용');
    expect(contents()).toContain('게시 시간 기준 영업 중');
    expect(contents()).toContain('예약 기능 있음 · 좌석 미확인');
    await press('다음 결과');
    expect(buttons('가게26')).not.toHaveLength(0);
    expect(buttons('예약 기능 없음')).toHaveLength(0);
    expect(buttons('시간 미게시')).toHaveLength(0);
    expect(buttons('시간 외')).toHaveLength(0);
    await press('지도');
    await Renderer.act(async () => {
      app.root.findAllByType(View).find(view => view.props.accessibilityLabel === '공개 지역 지도')!
        .props.onLayout({nativeEvent:{layout:{width:240,height:230}}});
    });
    expect(buttons('가게26')).not.toHaveLength(0);
    expect(buttons('가게1')).toHaveLength(0);
    await Renderer.act(async () => { app.root.findAllByType(Image)[0].props.onError(); });
    await press('목록 보기');
    result = 'offline';
    await press('공개 장소 정보 다시 조회');
    expect(buttons('가게26')).not.toHaveLength(0);
    result = 'stale';
    await press('서울 검색 다시 시도');
    expect(buttons('가게26')).not.toHaveLength(0);
    result = 'current';
    await press('서울 검색 다시 시도');
    expect(buttons('가게1')).not.toHaveLength(0);
    await press('필터 해제');
    await press('다음 결과');
    expect(contents()).toContain('영업시간 미확인');
    expect(buttons('예약 기능 없음')).not.toHaveLength(0);
    await Renderer.act(async () => {
      app.root.findAllByType(TextInput).find(input => input.props.accessibilityLabel === '장소 이름')!
        .props.onChangeText('없는 장소');
    });
    expect(contents()).toContain('조건에 맞는 공개 장소가 없습니다.');
    expect(buttons('필터')).not.toHaveLength(0);
    expect(permission).not.toHaveBeenCalled();
    expect(mockReadProfile).not.toHaveBeenCalled();
    expect(source.mock.calls.some(([, options]) => (options?.headers as Record<string, string>)?.['Cache-Control'] === 'no-cache')).toBe(true);
    expect(source.mock.calls.every(([url, options]) => !String(url).includes('booking=') &&
      (options as RequestInit).credentials === 'omit')).toBe(true);
  } finally {
    if (app) await Renderer.act(async () => app.unmount());
    source.mockRestore();
    clock.mockRestore();
  }
});

test('T01 discards superseded responses and rejects wrong-region or changed equal-version payloads', async () => {
  const activeRequests: {region:string; signal:AbortSignal; finish:(response:Response)=>void}[] = [];
  const fetcher = jest.spyOn(globalThis, 'fetch').mockImplementation((input, options) => {
    const url = String(input);
    if (url.endsWith('/v1/region-options')) {
      return Promise.resolve({ok:true,url,json:async () => [{code:'seoul', name:'서울'}, {code:'busan', name:'부산'}]} as Response);
    }
    return new Promise(resolve => activeRequests.push({region:url.endsWith('seoul') ? 'seoul' : 'busan', signal:options!.signal!,
      finish: response => resolve({...response,url})}));
  });
  const result = (region:string, name:string, version:number | string | undefined = 4) => ({
    ok: true, json: async () => ({environment:'public', version, places:[
      {region, id:'sample', name, address:'주소', source:'공개 자료', updatedAt:'2026-11-20T00:00:00Z'},
    ]}),
  }) as Response;
  let app!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {app = Renderer.create(<TravelHome back={() => {}} permission={async () => false} />);});
    const button = (label: string) => app.root.findAll(node => node.props.accessibilityLabel === label &&
      typeof node.props.onPress === 'function')[0];
    async function press(label:string) {
      expect(button(label)?.props.disabled).toBeFalsy();
      await Renderer.act(async () => { button(label).props.onPress(); });
    }
    const settle = (index:number, response:Response) => Renderer.act(async () => activeRequests[index].finish(response));
    await press('서울');
    await press('부산');
    expect(activeRequests[0].signal.aborted).toBe(true);
    await settle(1, result('busan','부산 승인 장소'));
    await settle(0, result('seoul','늦게 온 서울',5));
    expect(button('부산 승인 장소')).toBeDefined();
    expect(button('늦게 온 서울')).toBeUndefined();
    await press('공개 장소 정보 다시 조회');
    await settle(2, result('seoul','잘못된 지역 자료',5));
    expect(button('잘못된 지역 자료')).toBeUndefined();
    expect(button('부산 승인 장소')).toBeDefined();
    await press('부산 검색 다시 시도');
    await settle(3, result('busan','정밀도 초과',9007199254740992));
    expect(button('정밀도 초과')).toBeUndefined();
    await press('서울');
    await settle(4,result('seoul','서울 승인 장소'));
    expect(button('서울 승인 장소')).toBeDefined();
    await press('부산');
    await settle(5,result('busan','같은 버전 변조'));
    expect(button('같은 버전 변조')).toBeUndefined();
    expect(button('서울 승인 장소')).toBeDefined();
    await press('부산 검색 다시 시도');
    await settle(6,result('busan','부산 승인 장소'));
    expect(button('부산 승인 장소')).toBeDefined();
    await press('서울');
    await Renderer.act(async () => app.unmount());
    expect(activeRequests[7].signal.aborted).toBe(true);
    await settle(7,result('seoul','늦게 완료'));
  } finally {
    fetcher.mockRestore();
  }
});

test('the payment route opens existing bookings at the registered branch without public place search', async () => {
  mockHasEnrollment = true;
  mockBookingMode = true;
  mockCalls.length = 0;
  const publicSearch = jest.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('public search unavailable'));
  let app!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={async () => false}
        bookingScope={{branch: mockBranch, customer: mockBranch, tenant: mockBranch}} />);
    });
    expect(publicSearch).not.toHaveBeenCalled();
    expect(mockCalls).toEqual([`customer/my-bookings/${mockBranch}`]);
    expect(JSON.stringify(app.toJSON())).toContain('30.00 TestUSDC');
  } finally {
    mockHasEnrollment = false;
    mockBookingMode = false;
    publicSearch.mockRestore();
    await Renderer.act(async () => { app?.unmount(); });
  }
});

test.each(['customer', 'branch'])('the payment booking route drops a late response after the %s changes', async changed => {
  mockHasEnrollment = true;
  mockBookingMode = true;
  mockCalls.length = 0;
  let release!: () => void;
  mockBookingDelay = new Promise(resolve => { release = resolve; });
  const publicSearch = jest.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('public search unavailable'));
  let notify: ((state: AppStateStatus) => void) | undefined;
  const listen = jest.spyOn(AppState, 'addEventListener').mockImplementation((_, handler) => {
    notify = handler;
    return {remove: () => {}};
  });
  let app!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      app = Renderer.create(<TravelHome back={() => {}} permission={async () => false}
        bookingScope={{branch: mockBranch, customer: mockBranch, tenant: mockBranch}} />);
    });
    expect(mockCalls).toEqual([`customer/my-bookings/${mockBranch}`]);
    await Renderer.act(async () => { notify?.('background'); });
    const different = '0fb9757b-d626-4fb8-b02e-68a9fc8930f0';
    if (changed === 'customer') mockBookingIdentity = different;
    else mockRegisteredBranch = different;
    await Renderer.act(async () => { notify?.('active'); });
    await Renderer.act(async () => { release(); });
    const contents = JSON.stringify(app.toJSON());
    expect(contents).not.toContain('30.00 TestUSDC');
    expect(contents).toContain('이 계정에서 예약을 열 수 없습니다.');
    expect(mockCalls).toHaveLength(1);
  } finally {
    release();
    mockBookingDelay = undefined;
    mockBookingIdentity = mockBranch;
    mockRegisteredBranch = mockBranch;
    mockHasEnrollment = false;
    mockBookingMode = false;
    publicSearch.mockRestore();
    listen.mockRestore();
    await Renderer.act(async () => { app?.unmount(); });
  }
});
