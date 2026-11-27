import {readFileSync} from 'fs';
import {join} from 'path';
import React from 'react';
import Renderer from 'react-test-renderer';
import {Text} from 'react-native';
import {editionOf, missingConsents, noteConsents, orderAlertsOff, requiredConsents, validConsents, withConsent} from '../src/legal/consent';
import {LegalHome} from '../src/legal/LegalScreens';
import {notices, type Notice} from '../src/legal/notices';
import {appVersion} from '../src/ui/version';

jest.unmock('../src/legal/ConsentGate.tsx');
let mockStored = '', mockRefuse = false;
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => mockStored,
  compareAndSaveProfile: async (expected: string, next: string) => { if (mockRefuse) throw new Error('full'); if (expected !== mockStored) return false; mockStored = next; return true; }}}));

let tree: Renderer.ReactTestRenderer;
const flush = () => Renderer.act(async () => { for (let i = 0; i < 30; i++) await Promise.resolve(); });
const text = () => JSON.stringify(tree.toJSON());
const pick = (label: string) => tree.root.findAll(node => typeof node.props.accessibilityLabel === 'string' && node.props.accessibilityLabel.startsWith(label) && typeof node.props.onPress === 'function')[0];
const tap = async (label: string) => { expect([label, !!pick(label), !!pick(label)?.props.disabled]).toEqual([label, true, false]); await Renderer.act(async () => { pick(label).props.onPress(); }); await flush(); };
async function open() {
  const {ConsentGate} = require('../src/legal/ConsentGate');
  await Renderer.act(async () => { tree = Renderer.create(<ConsentGate><Text>지갑 안</Text></ConsentGate>); });
  await flush();
}
beforeEach(() => { mockStored = ''; mockRefuse = false; noteConsents([]); });
afterEach(async () => { await Renderer.act(async () => tree?.unmount()); });

test('every required document exists in this build, in Korean', () => {
  for (const id of requiredConsents) expect([id, notices.documents.some(item => item.id === id && item.language === 'ko')]).toEqual([id, true]);
});
test('nothing opens before all required agreements, and they are kept with their edition', async () => {
  await open();
  expect(text()).not.toContain('지갑 안');
  expect(pick('동의하고 시작').props.disabled).toBe(true);
  const labels = requiredConsents.map(id => notices.documents.find(item => item.id === id && item.language === 'ko')!.title);
  await tap(labels[0]); await tap(labels[1]);
  expect(pick('동의하고 시작').props.disabled).toBe(true);
  await tap(labels[2]);
  mockRefuse = true;
  await tap('동의하고 시작');
  expect(text()).toContain('동의를 저장하지 못했습니다');
  mockRefuse = false;
  await tap('동의하고 시작');
  expect(text()).toContain('지갑 안');
  const kept = JSON.parse(mockStored).consents;
  expect(kept.map((item: {id: string}) => item.id).sort()).toEqual(['alerts', ...requiredConsents].sort());
  expect(kept.find((item: {id: string}) => item.id === 'alerts').declined).toBe(true);
  expect(orderAlertsOff()).toBe(true);
  await Renderer.act(async () => tree.unmount());
  await open();
  expect(text()).toContain('지갑 안');
});
test('a document can be read before agreeing, also in English', async () => {
  await open();
  const title = notices.documents.find(item => item.id === 'terms' && item.language === 'ko')!;
  await tap(`${title.title} (필수) 보기`);
  expect(text()).toContain(title.sections[0].heading);
  await tap('돌아가기');
  await tap('English');
  expect(text()).toContain('Before you start');
});
test('a new edition is asked again', () => {
  const documents = requiredConsents.map((id): Notice => ({id, language: 'ko', title: id, edition: '1', status: 'draft', effective: null, sections: []}));
  let given = requiredConsents.reduce((list, id) => withConsent(list, id, true, 5, documents), [] as ReturnType<typeof withConsent>);
  expect(missingConsents(given, documents)).toEqual([]);
  expect(missingConsents(given, documents.map(item => item.id === 'privacy' ? {...item, edition: '2'} : item))).toEqual(['privacy']);
  expect(withConsent(given, 'terms', false, 6, documents)).toBe(given);
  given = withConsent(withConsent(given, 'alerts', false, 6), 'alerts', true, 7);
  expect(given.filter(item => item.id === 'alerts')).toEqual([{id: 'alerts', edition: editionOf('alerts'), at: 7}]);
  expect(validConsents(given)).toBe(true);
  for (const bad of [{}, [{id: 'terms'}], [{id: 'x', edition: '1', at: 1}], [{id: 'terms', edition: '1', at: 1, declined: true}], [given[0], given[0]]]) expect(validConsents(bad)).toBe(false);
});
test('legal notices list the documents, the licences and the agreements', async () => {
  const alerts: boolean[] = [];
  await Renderer.act(async () => { tree = Renderer.create(<LegalHome language="ko" consents={[{id: 'terms', edition: '1', at: 1_800_000_000_000}]} setAlerts={on => { alerts.push(on); }} busy={false} back={() => {}} />); });
  for (const item of notices.documents.filter(document => document.language === 'ko')) expect(pick(item.title)).toBeDefined();
  await tap('주문 준비 알림');
  expect(alerts).toEqual([true]);
  await tap('오픈소스 라이선스');
  expect(text()).toContain('앱에 들어간 패키지');
  if (notices.licenses.length) {
    await tap(notices.licenses[0].name);
    expect(notices.texts[notices.licenses[0].text].length).toBeGreaterThan(100);
    expect(text()).toContain(notices.licenses[0].version);
  }
});
test('the version shown is the version built', () => {
  expect(readFileSync(join(__dirname, '../android/app/build.gradle'), 'utf8')).toContain(`versionName "${appVersion}"`);
});
