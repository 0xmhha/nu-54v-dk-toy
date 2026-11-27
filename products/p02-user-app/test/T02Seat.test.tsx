import {TextInput} from 'react-native';
import Renderer from 'react-test-renderer';
import {TravelHome} from '../src/travel/TravelHome.tsx';

const mockBranch = 'd6d10219-1c27-492c-9bdf-ec14d7d092bd';
const mockAsset = '0x' + 'a'.repeat(40);
const mockOther = 'd58a59db-25fe-4d17-a2a0-d50f28362399';
const mockID = '92902f3f-51a6-4358-96b5-7f49478d86ae';
const mockJournal = new Map<string,string>();
type Operation = {operation:string; branch:string; slot:string; requestKey:string; partySize:number;
  termsVersion:number; asset:string; deposit:string; balance:string};
const mockCalls: Operation[] = [];
let mockSubject = mockBranch;
let mockActiveBranch = mockBranch;
let mockOutcome: 'sold_out'|'terms_changed'|'lost'|'accepted';
let mockCurrentOptions = false;
let mockRandomCounter = 0;
jest.mock('../src/travel/publicDeployment.ts', () => ({approvedTravelOrigin: 'https://places.example'}));

jest.mock('../src/specs/NativeRenterBle.ts', () => ({
  __esModule: true,
  default: {loadDevices: async () => ''},
}));

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadJournal: async (name:string) => mockJournal.get(name) ?? '',
    saveJournal: async (name:string,value:string) => {mockJournal.set(name,value);},
    loadProfile: async () => {
      return JSON.stringify({
      scope: {role: 'customer', branchRef: mockActiveBranch, subjectRef: mockSubject, tenantRef: mockBranch,
        expiresAt: Math.floor(Date.now() / 1000) + 120},
      pin: 'a'.repeat(64),
      payment: {chain: 8283, contract: '0x' + 'b'.repeat(40), beneficiary: '0x' + 'c'.repeat(40),
        signer: '0x' + 'd'.repeat(40), asset: mockAsset, merchantId: 'e'.repeat(64), keyVersion: 1,
        beneficiaryVersion: 1, maxBaseUnits: '30000000'},
    });},
    randomBytes: async () => (++mockRandomCounter===1 ? 'AAAAAAAAAAAAAAAAAAAAAA==' : 'AQAAAAAAAAAAAAAAAAAAAA=='),
    exchange: async (_slot: string, command: string) => {
      const {travel} = JSON.parse(command) as {travel:Operation};
      mockCalls.push(travel);
      if (travel.operation==='hold') {
        expect([...mockJournal.values()].some(value=>value.includes(travel.requestKey))).toBe(true);
        if (mockOutcome==='lost') throw new Error('native timeout');
        if (mockOutcome!=='accepted') return JSON.stringify({error:mockOutcome});
        return JSON.stringify({booking:{id:mockID, state:'held', cancellation:'새 취소 조건',
          expiresAt:'2026-12-01T12:00:00Z', ...travel}});
      }
      if (travel.operation==='legs') return '{"legs":[]}';
      if (travel.operation!=='options') throw new Error(travel.operation);
      return JSON.stringify({slots: [{id:mockCurrentOptions ? mockOther : mockBranch,
        termsVersion:mockCurrentOptions ? 2 : 1, remaining:3,
        startsAt:mockCurrentOptions ? '2026-12-01T13:00:00Z' : '2026-12-01T12:00:00Z',
        cancellation:mockCurrentOptions ? '새 취소 조건' : '이전 취소 조건',
        asset:mockAsset, deposit:mockCurrentOptions ? '12000000' : '10000000', balance:'20000000',
      }]});
    },
  },
}));

let app: Renderer.ReactTestRenderer | undefined;
let source: jest.SpyInstance;

beforeEach(() => {
  mockCalls.length=0;
  mockJournal.clear();
  mockSubject=mockBranch;
  mockActiveBranch=mockBranch;
  mockCurrentOptions=false;
  mockRandomCounter=0;
  const place={region:'seoul',address:'서울',source:'시험 자료',updatedAt:'2026-11-20T00:00:00Z',
    branchRef:mockBranch,capabilities:['booking']};
  source=jest.spyOn(globalThis,'fetch').mockImplementation(async input => {
    const url=String(input);
    return {ok:true,url,json:async()=>url.endsWith('/v1/region-options') ?
      [{code:'seoul',name:'서울'}] : {environment:'synthetic',places:[
        {...place,id:'dining',name:'한식당'},{...place,id:'annex',name:'별관'},
        {...place,id:'other-branch',name:'다른 지점',branchRef:mockOther},
      ]}} as Response;
  });
});

afterEach(async () => {
  source.mockRestore();
  if(app) await Renderer.act(async()=>{app!.unmount();app=undefined;});
});

function button(label: string) {
  return app!.root.findAll(node=>node.props.accessibilityLabel===label && typeof node.props.onPress==='function')[0];
}

async function press(label: string) {
  const target=button(label);
  expect(target).toBeDefined();
  expect(target.props.disabled).toBeFalsy();
  await Renderer.act(async()=>{target.props.onPress();});
}

async function openPlace(name='한식당') {
  await Renderer.act(async()=>{app=Renderer.create(<TravelHome back={()=>{}} permission={async()=>false}/>);});
  await press('서울');
  await press(name);
  await press('예약 가능한 시간');
}

async function chooseTwo() {
  await Renderer.act(async()=>{
    app!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='인원')!.props.onChangeText('2');
  });
}
const commands=()=>mockCalls.filter(call=>call.operation==='hold');
const contents=()=>JSON.stringify(app!.toJSON());

test.each([
  ['sold_out','다른 시간 선택','선택한 시간이 마감됐어요'],
  ['terms_changed','최신 조건 확인','예약 조건이 바뀌었어요'],
] as const)('known %s unlocks a new review following a fresh options read',async (outcome,recheck,notice)=>{
  mockOutcome=outcome;
  await openPlace();
  await chooseTwo();
  expect(contents()).toContain('60.00 TestUSDC');
  await press('이 조건으로 예약 보류');
  expect(contents()).toContain(notice);
  expect(button('같은 예약 요청 다시 시도')).toBeUndefined();
  expect(button('이 조건으로 예약 보류')).toBeUndefined();
  expect(button('예약 가능한 시간').props.disabled).toBeFalsy();
  expect([...mockJournal.values()]).toEqual(['']);
  mockCurrentOptions=true;
  await press(recheck);
  expect(contents()).toContain('새 취소 조건');
  expect(contents()).toContain('64.00 TestUSDC');
  expect(commands()).toHaveLength(1);
  mockOutcome='accepted';
  await press('이 조건으로 예약 보류');
  expect(commands()).toHaveLength(2);
  expect(commands()[1]).toMatchObject({slot:mockOther,partySize:2,termsVersion:2,deposit:'24000000'});
  expect(commands()[1].requestKey).not.toBe(commands()[0].requestKey);
  expect(contents()).toContain('예약 보류');
});

test('a lost reply is restored after restart only in the original place and customer registration', async () => {
  mockOutcome='lost';
  await openPlace();
  await chooseTwo();
  await press('이 조건으로 예약 보류');
  const original=commands()[0];
  expect(button('예약 가능한 시간').props.disabled).toBe(true);
  expect(button('같은 예약 요청 다시 시도')).toBeDefined();
  await Renderer.act(async()=>{app!.unmount();app=undefined;});
  await openPlace('별관');
  expect(contents()).toContain('처음 요청한 장소');
  expect(button('같은 예약 요청 다시 시도')).toBeUndefined();
  expect(button('이 조건으로 예약 보류')).toBeUndefined();
  await Renderer.act(async()=>{app!.unmount();app=undefined;});
  mockActiveBranch=mockOther;
  await openPlace('다른 지점');
  expect(button('같은 예약 요청 다시 시도')).toBeUndefined();
  expect(commands()).toEqual([original]);
  await Renderer.act(async()=>{app!.unmount();app=undefined;});
  mockActiveBranch=mockBranch;
  mockSubject=mockOther;
  await openPlace();
  expect(button('같은 예약 요청 다시 시도')).toBeUndefined();
  expect(commands()).toEqual([original]);
  await Renderer.act(async()=>{app!.unmount();app=undefined;});
  mockSubject=mockBranch;
  await openPlace();
  expect(contents()).toContain('조건 버전');
  expect(contents()).toContain('20.00 TestUSDC');
  expect(contents()).toContain('40.00 TestUSDC');
  expect(button('이 조건으로 예약 보류')).toBeUndefined();
  mockOutcome='accepted';
  await press('같은 예약 요청 다시 시도');
  expect(commands()).toEqual([original,original]);
  expect(mockRandomCounter).toBe(1);
  expect([...mockJournal.values()]).toEqual(['']);
  expect(contents()).toContain('예약 보류');
});
