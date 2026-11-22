import Renderer from 'react-test-renderer';
import {TextInput} from 'react-native';
import {TravelPrivate} from '../src/travel/TravelPrivate.tsx';

const expiry = new Date(Date.now()+7*86_400_000).toISOString();
const mockCalls = jest.fn<Promise<string>,[string,string,string]>();
const policy = {purpose:'recommendation',version:1,recipient:'p09',maxLifetimeSeconds:604800,targetRequired:false};
const grant = {purpose:'recommendation', revision:1, policyVersion:1,recipient:'p09',accepted:true,expiresAt:expiry,
  receipt:'5a613e0c-85a1-429a-8550-e54a87cafd51'};
const park={id:'park',name:'공원',address:'서울',source:'fixture',updatedAt:expiry,region:'seoul'};

jest.mock('../src/specs/NativeCommerceHttp.ts',()=>({
  __esModule:true,
  default:{
    loadProfile:async()=>JSON.stringify({scope:{role:'private-user',expiresAt:Date.now()/1000+300},
      policies:[{purpose:'recommendation',version:1,recipient:'p09',maxLifetimeSeconds:604800,targetRequired:false}]}),
    exchangeTravel:(method:string,path:string,body:string)=>mockCalls(method,path,body),
    randomBytes:async()=> 'AAAAAAAAAAAAAAAAAAAAAA==',
    loadJournal:async()=> '',
    discardTravelRecording:async()=>{},
  },
}));

test('pre-consent draft is sent once only after explicit approval and late canceled results stay closed', async()=>{
  let accepted=false;
  let offer!: (response:string)=>void;
  mockCalls.mockImplementation(async(method,path,body)=>{
    if(path==='/consents') {
      if(method==='POST') {
        expect(JSON.parse(body)).toMatchObject({purpose:policy.purpose,recipient:policy.recipient,accept:true});
        expect(body).not.toContain('23000');
        accepted=true;
        return JSON.stringify({receipt:grant.receipt});
      }
      return JSON.stringify({consents:accepted?[grant]:[]});
    }
    if(path==='/records') return JSON.stringify({records:[],itineraries:[]});
    if(path==='/erasure') return '{"receipts":[]}';
    if(path==='/itineraries/generate') return new Promise(resolve=>{offer=resolve;});
    if(path==='/itineraries/apply') return '{"revision":1}';
    throw Error(path);
  });
  let tree!: Renderer.ReactTestRenderer;
  function find(label:string) {return tree.root.findAll(n=>n.props.accessibilityLabel===label && typeof n.props.onPress==='function')[0];}
  async function tap(label:string) {
    const button=find(label); expect(button).toBeDefined(); expect(button.props.disabled).toBeFalsy();
    await Renderer.act(async()=>{button.props.onPress();});
  }
  const calls=(path:string)=>mockCalls.mock.calls.filter(call=>call[1]===path);
  const budget=()=>tree.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='일정 예산 상한 (원)')!;
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<TravelPrivate place={park} places={[park]} catalogVersion={1} back={()=>{}}/>);});
    await tap('내 일정과 추천');
    await tap('다음 날짜');
    await Renderer.act(async()=>{budget().props.onChangeText('23000');});
    await tap('추천 동의 확인');
    expect(calls('/itineraries/generate')).toHaveLength(0);
    await tap('동의하지 않음');
    expect(budget().props.value).toBe('23000');
    await tap('추천 동의 확인');
    await tap('동의하고 생성 요청');
    expect(calls('/itineraries/generate')).toHaveLength(1);
    const sent=JSON.parse(calls('/itineraries/generate')[0][2]);
    expect(sent).toMatchObject({budgetLimit:'23000',consentRevision:1,region:'seoul'});
    expect(Date.parse(sent.end)-Date.parse(sent.start)).toBe(4*3600_000);
    await tap('추천 생성 취소');
    await Renderer.act(async()=>{offer(JSON.stringify({provider:'fixture',confirmation:'ok',preview:{...sent,stops:[]}}));});
    expect(find('검토한 일정 적용')).toBeUndefined();
    expect(calls('/itineraries/apply')).toHaveLength(0);
    expect(budget().props.value).toBe('23000');
    await tap('날짜와 선호로 코스 추천');
    expect(calls('/itineraries/generate')).toHaveLength(2);
    const retried=JSON.parse(calls('/itineraries/generate')[1][2]);
    expect(retried).toEqual(sent);
    await Renderer.act(async()=>{offer(JSON.stringify({provider:'fixture',confirmation:'reviewed',preview:{...retried,stops:[]}}));});
    expect(calls('/itineraries/apply')).toHaveLength(0);
    await tap('검토한 일정 적용');
    expect(JSON.parse(calls('/itineraries/apply')[0][2])).toMatchObject({consentRevision:1,
      budgetLimit:'23000',confirmedPreview:'reviewed'});
  } finally {if(tree)await Renderer.act(async()=>{tree.unmount();});}
});
