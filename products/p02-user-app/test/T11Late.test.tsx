import Renderer from 'react-test-renderer';
import {AppState} from 'react-native';
import {TravelPrivate} from '../src/travel/TravelPrivate.tsx';
import {TravelItinerary} from '../src/travel/TravelItinerary.tsx';
import type {Consent} from '../src/travel/privateClient.ts';

const plan = '43a668b5-142a-4e04-b178-bd736c30779d';
const receipt = '5a613e0c-85a1-429a-8550-e54a87cafd51';
const mockExchange = jest.fn<Promise<string>, [string, string, string]>();
const mockRandom = jest.fn(async () => 'AAAAAAAAAAAAAAAAAAAAAA==');
const mockProfile = jest.fn<Promise<string>, [string]>();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: (scope: string) => mockProfile(scope),
    exchangeTravel: (method: string, path: string, envelope: string) => mockExchange(method, path, envelope),
    randomBytes: () => mockRandom(),
    loadJournal: async () => '',
    discardTravelRecording: async () => {},
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

const expiry = new Date(Date.now() + 3 * 86_400_000).toISOString();

beforeEach(() => {
  mockExchange.mockReset();
  mockRandom.mockReset().mockResolvedValue('AAAAAAAAAAAAAAAAAAAAAA==');
  mockProfile.mockReset();
  mockProfile.mockResolvedValue(JSON.stringify({scope: {role: 'private-user', expiresAt: Date.now() / 1000 + 300},
    policies: [{purpose: 'recommendation', version: 1, recipient: 'p09', maxLifetimeSeconds: 1200, targetRequired: false}]}));
});


const park = {id: 'park', name: '공원', address:'서울', source:'fixture', updatedAt:expiry, region:'seoul',
    opening:[{weekday:1, timezone:'Asia/Seoul', opens:'09:00', closes:'18:00'}]};
const consent: Consent = {purpose:'recommendation', revision:1, policyVersion:1,recipient:'p09',accepted:true,expiresAt:expiry,receipt};

test.each([undefined, {...consent, revision: 2}, {...consent, receipt: plan}])('obsolete provider response cannot open its preview (%j)', async replacement => {
  let deliver!: (reply:string) => void;
  mockExchange.mockImplementation(async (_,path) => {
    if(path==='/records') return '{"itineraries":[]}';
    if(path==='/itineraries/generate') return new Promise(r=>{deliver=r;});
    throw new Error(path);
  });
  const screen=(allowed?:Consent)=><TravelItinerary place={park} places={[park]} catalogVersion={1} consent={allowed}/>;
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(screen(consent));});
    await press(tree,'날짜와 선호로 코스 추천');
    expect(typeof deliver).toBe('function');
    await Renderer.act(async()=>{tree.update(screen(replacement));});
    await Renderer.act(async()=>{deliver(JSON.stringify({provider:'public-fixture-provider',confirmation:'synthetic',preview:{
       id:plan,consentRevision:1,catalogVersion:1,expiresAt:expiry,budgetLimit:'15000',stops:[]
    }}));});
    const review=tree.root.findAll(n=>n.props.accessibilityLabel==='검토한 일정 적용' && typeof n.props.onPress==='function');
    expect(review.length).toBe(0);
  } finally {if(tree)await Renderer.act(async()=>{tree.unmount();});}
});

test('expiry during apply intent creation hides the preview and prevents the next request', async () => {
  jest.useFakeTimers();
  const lifetime = 3 * 86_400_000;
  const expiresAt = new Date(Date.now()+lifetime).toISOString();
  const grant = {...consent, expiresAt};
  let finishKey!: (value: string) => void;
  mockExchange.mockImplementation(async (_,path) => {
    if(path==='/records') return '{"itineraries":[]}';
    if(path==='/itineraries/generate') return JSON.stringify({provider:'fixture', confirmation:'confirm',
      preview:{id:plan, consentRevision:1, catalogVersion:1, expiresAt, budgetLimit:'15000', stops:[]}});
    throw new Error(path);
  });
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<TravelItinerary place={park} places={[park]} catalogVersion={1} consent={grant}/>);});
    await press(tree,'날짜와 선호로 코스 추천');
    mockRandom.mockImplementationOnce(()=>new Promise(resolve=>{finishKey=resolve;}));
    await press(tree,'검토한 일정 적용');
    expect(typeof finishKey).toBe('function');
    await Renderer.act(async()=>{jest.advanceTimersByTime(lifetime);finishKey('AAAAAAAAAAAAAAAAAAAAAA==');});
    expect(mockExchange.mock.calls.some(call=>call[1]==='/itineraries/apply')).toBe(false);
    expect(tree.root.findAll(node=>node.props.accessibilityLabel==='검토한 일정 적용')).toHaveLength(0);
  } finally {
    if(tree) await Renderer.act(async()=>{tree.unmount();});
    jest.useRealTimers();
  }
});

test.each(['resume', 'reload'])('withdrawal on another device closes the existing review on %s', async trigger => {
  let change!: (state: 'background'|'active') => void;
  const listener=jest.spyOn(AppState,'addEventListener').mockImplementation((_, callback)=>{
    change=callback;return {remove:jest.fn()};
  });
  let latest=consent;
  mockExchange.mockImplementation(async (method,path) => {
    if(path==='/consents') return JSON.stringify({consents:[latest]});
    if(path==='/records') return method==='POST' ? '{"itineraries":[]}' : '{"records":[]}';
    if(path==='/erasure') return '{"receipts":[]}';
    if(path==='/itineraries/generate') return JSON.stringify({provider:'fixture', confirmation:'confirm',
      preview:{id:plan, consentRevision:1, catalogVersion:1,expiresAt:expiry,budgetLimit:'15000',stops:[]}});
    throw new Error(path);
  });
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<TravelPrivate back={()=>{}} place={park} places={[park]} catalogVersion={1}/>);});
    await press(tree,'내 일정과 추천');
    await press(tree,'날짜와 선호로 코스 추천');
    findButton(tree,'검토한 일정 적용');
    latest={...consent,revision:2,accepted:false};
    if (trigger==='resume') {
      await Renderer.act(async()=>{change('background');change('active');});
    } else {
      await press(tree,'저장한 일정 다시 불러오기');
    }
    expect(tree.root.findAll(node=>node.props.accessibilityLabel==='검토한 일정 적용')).toHaveLength(0);
    expect(mockExchange.mock.calls.filter(call=>call[0]==='GET' && call[1]==='/consents')).toHaveLength(2);
    expect(mockExchange.mock.calls.filter(call=>call[0]==='POST' && call[1]==='/records')).toHaveLength(1);
  } finally {
    if(tree) await Renderer.act(async()=>{tree.unmount();});
    listener.mockRestore();
  }
});

test('leaving the itinerary prevents a pending ID from starting a proposal', async () => {
  let resolveID!: (value: string) => void;
  mockRandom.mockImplementationOnce(()=>new Promise(resolve=>{resolveID=resolve;}));
  mockExchange.mockResolvedValue('{"itineraries":[]}');
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{
      tree=Renderer.create(<TravelItinerary place={park} places={[park]} catalogVersion={1} consent={consent}/>);
    });
    await press(tree,'날짜와 선호로 코스 추천');
    expect(typeof resolveID).toBe('function');
    await Renderer.act(async()=>{tree.unmount();});
    await Renderer.act(async()=>{resolveID('AAAAAAAAAAAAAAAAAAAAAA==');});
    expect(mockExchange.mock.calls.map(call=>call[1])).toEqual(['/records']);
  } finally {
    if(tree) await Renderer.act(async()=>{tree.unmount();});
  }
});

test('an old successful apply cannot refresh records through a newer consent session', async () => {
  let finishApply!: (reply: string) => void;
  mockExchange.mockImplementation(async (_,path) => {
    if(path==='/records') return '{"itineraries":[]}';
    if(path==='/itineraries/generate') return JSON.stringify({provider:'fixture', confirmation:'confirm',
      preview:{id:plan, consentRevision:1, catalogVersion:1,expiresAt:expiry,budgetLimit:'15000',stops:[]}});
    if(path==='/itineraries/apply') return new Promise(resolve=>{finishApply=resolve;});
    throw new Error(path);
  });
  const screen=(grant: Consent)=><TravelItinerary place={park} places={[park]} catalogVersion={1} consent={grant}/>;
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(screen(consent));});
    await press(tree,'날짜와 선호로 코스 추천');
    await press(tree,'검토한 일정 적용');
    expect(typeof finishApply).toBe('function');
    await Renderer.act(async()=>{tree.update(screen({...consent,revision:3}));});
    await Renderer.act(async()=>{finishApply('{"revision":1}');});
    expect(mockExchange.mock.calls.map(call=>call[1])).toEqual([
      '/records','/itineraries/generate','/itineraries/apply','/records',
    ]);
    expect(tree.root.findAll(node=>node.props.accessibilityRole==='alert')).toHaveLength(0);
  } finally {
    if(tree) await Renderer.act(async()=>{tree.unmount();});
  }
});
