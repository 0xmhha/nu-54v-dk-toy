import React, {useMemo, useState} from 'react';
import {ScrollView, StyleSheet, Text, View} from 'react-native';
import {Brand, KitButton, Section} from '../ui/kit.tsx';
import {color, radius, font} from '../ui/theme.ts';
import {defaultReadState, observationScope, type ReadProfileState} from './readProfiles.ts';
import {OrderPayment, type OrderPaymentView} from '../merchant/OrderPayment.tsx';
import {OpenOrderCard, OrderDetail} from '../merchant/OpenOrders.tsx';
import type {OpenOrder} from '../merchant/orderRecords.ts';
import {BoardLife, boardLifeTitles, SetupStatus, type BoardLifeProps, type BoardLifeScreen} from './BoardLife.tsx';
import {storeMoney} from './tokens.ts';
import {BoardCard, BoardPage} from './HomeParts.tsx';
import {WalletHomeSummary} from './WalletHomeSummary.tsx';

// Fixture-only wallet home for design review on a QA build (OwnerWallet.demoWallet()). It never
// reads or writes the stored profile, never connects to a board and every action is disabled, so
// nothing here can approve, sign or send. The banner stays on screen the whole time.
const demoOwner = {boardFingerprint: 'de'.repeat(32), generation: 1, address: '0x' + 'de'.repeat(20)};

export function demoReadState(now: number): ReadProfileState {
  const selection = defaultReadState.selection;
  return {...defaultReadState, observations: [{
    address: demoOwner.address, scope: observationScope(demoOwner.boardFingerprint, demoOwner.generation, selection),
    time: now, point: '시연 데이터', amount: '42000000', decimals: 6, unit: 'TestUSDC',
  }]};
}

/** Fixture states of an order payment. Nothing behind them can reach a board, a kiosk or a server. */
export function demoPayments(now: number): [string, OrderPaymentView][] {
  const base = {store: '그린 카페', amount: '4.50 TestUSDC'};
  const board = {perPayment: '30.00 TestUSDC', left: '95.50 TestUSDC', balance: '42.00 TestUSDC'};
  const confirm = {...base, kind: 'confirm' as const, deadline: now + 102, items: [{label: '라떼 · 아이스 ×1', amount: '4.50 TestUSDC'}]};
  return [
    ['결제 확인', {...confirm, board}], ['보드 찾기 전', confirm], ['보드 승인 대기', {...base, kind: 'waiting', deadline: now + 84}],
    ['잔액 부족', {...confirm, board: {...board, balance: '1.20 TestUSDC'}, block: 'balance'}],
    ['한도 초과', {...confirm, amount: '45.00 TestUSDC', items: [], board, block: 'over-limit'}],
    ['오늘 한도 초과', {...confirm, board: {...board, left: '3.00 TestUSDC'}, block: 'over-daily'}],
    ['지원하지 않는 주문', {...base, kind: 'confirm', items: [], block: 'unsupported'}],
    ['결제 거절', {...base, kind: 'stopped', reason: 'refused', until: now + 64}],
    ['승인 뒤 취소', {...base, kind: 'stopped', reason: 'cancelled', until: now + 64, spent: true}], ['결제 보내는 중', {...base, kind: 'submitting'}],
    ['결과 확인 중', {...base, kind: 'unknown'}], ['결제 완료', {...base, kind: 'done', number: 17}], ['결제 안 됨', {...base, kind: 'failed'}],
  ];
}

export function demoOrders(now: number): [string, OpenOrder][] {
  const base: OpenOrder = {payer: '0x' + 'de'.repeat(20), generation: '1', sale: '', branch: '', customer: '', confirmer: '0x' + 'de'.repeat(20),
    escrow: '0x' + 'de'.repeat(20), merchantId: 'de'.repeat(32), orderKey: 'de'.repeat(32), asset: '', amount: '4500000', sentAt: now - 1800,
    store: '그린 카페', stage: 'locked', pickupNo: 17};
  const delivered = {...base, stage: 'delivered' as const, pickup: 'collected' as const, claimableAt: now + 161_000};
  return [
    ['주문 접수', base], ['주문 준비 중', {...base, pickup: 'preparing'}], ['주문 준비 완료', {...base, pickup: 'ready'}],
    ['주문 전달됨', delivered], ['답 확인 중', {...delivered, pendingAction: 'receipt'}], ['주문 이의', {...delivered, stage: 'disputed', resolveBy: now + 900_000}],
    ['주문 완료', {...delivered, stage: 'paid', payout: 'receipt'}], ['주문 환불', {...base, stage: 'refunded', refund: 'store', pickup: 'unfulfillable'}],
  ];
}

type DemoLife = {screen: BoardLifeScreen; stored: BoardLifeProps['stored']; fixture: NonNullable<BoardLifeProps['fixture']>; found?: BoardLifeProps['found']; seen?: true};
/** Fixture states of setup, deposit, return and reset. Every action is off. */
export function demoLife(now: number): [string, DemoLife][] {
  const verified = {...demoOwner, network: '8283' as const};
  const board = {device: demoOwner.boardFingerprint, account: demoOwner.address, generation: 1, testedAt: now};
  const home = '0x' + 'ab'.repeat(20), device = 'de'.repeat(32), nonce = 'de'.repeat(32);
  const fresh = {version: 2 as const, exercised: false, mode: 'personal' as const};
  const tested = {...fresh, verified, spendingBoard: board}, full = {...tested, spendingBoard: {...board, returnAddress: home}};
  const balance = {units: 42_000_000n, latest: 42_000_000n, text: '42.00 TestUSDC'}, found = [{address: 'demo', name: 'NU54'}];
  const record = {payer: demoOwner.address, generation: '1', device, token: '0x' + 'de'.repeat(20), to: home, amount: '42000000',
    validBefore: now + 400, nonce, key: 'dededede-dede-4ede-9ede-dededededede', signature: 'de'.repeat(64) + '1b'};
  const back = (stage: 'signing' | 'sent' | 'final' | 'resetting') => ({screen: 'return' as const, stored: full, found, fixture: {balance, record: {...record, stage}}});
  return [
    ['설정 시작', {screen: 'setup', stored: fresh, fixture: {}}], ['설정 이어서', {screen: 'setup', stored: tested, found, fixture: {}}],
    ['반납 주소 입력', {screen: 'return-address', stored: tested, found, fixture: {}}], ['입금 안내', {screen: 'deposit', stored: full, fixture: {balance}, seen: true}], ['입금 주소 숨김', {screen: 'deposit', stored: full, found, fixture: {balance}}],
    ['입금 잠김', {screen: 'deposit', stored: fresh, fixture: {}}], ['반납 시작 전', {screen: 'return', stored: full, fixture: {balance}}],
    ['반납 막힘', {screen: 'return', stored: full, fixture: {balance, open: 1}}], ['반납 보드 승인', back('signing')], ['반납 확인 중', back('sent')],
    ['반납 확정', back('final')], ['초기화 확인 중', back('resetting')], ['초기화 안내', {screen: 'reset', stored: full, fixture: {balance}}], ['초기화 막힘', {screen: 'reset', stored: full, fixture: {balance, open: 1}}],
    ['운영자 초기화 요청', {screen: 'operator-reset', stored: full, fixture: {request: {device, generation: 1n, nonce, until: now + 600}}}],
  ];
}

export function DemoWalletHome({t, close}: {t: (ko: string) => string; close: () => void}) {
  const [hidden, setHidden] = useState(false);
  const base = useMemo(() => demoReadState(Date.now()), []);
  const state = {...base, selection: {...base.selection, hidden}};
  const off = () => {};
  const [shown, setShown] = useState<OrderPaymentView | 'board' | 'permit' | OpenOrder | DemoLife>();
  if (shown) {
    const leave = () => setShown(undefined);
    return <ScrollView contentContainerStyle={styles.content} testID="demo-page">
      <Text style={styles.ribbon}>{t('시연용 화면')}</Text>
      {typeof shown === 'object' && 'fixture' in shown ? <><Text accessibilityRole="header" style={styles.title}>{boardLifeTitles[shown.screen]}</Text>
        <BoardLife screen={shown.screen} stored={shown.stored} fixture={shown.fixture} found={shown.found ?? []} native={null} hasEnrollment={!!shown.stored.verified} busy={false}
          spending={{account: demoOwner.address, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 300, orderOpen: false, sends: true, leftToday: 95_500_000n}}
          scan={off} go={leave} orders={off} work={off} say={off} withBoard={() => Promise.reject(new Error('DEMO'))} wait={off} commit={async () => {}} copy={off} seen={off}
          seenBoard={shown.seen ? {account: demoOwner.address, generation: 1, at: Date.now()} : undefined} /></> :
        shown === 'permit' ? <><Text accessibilityRole="header" style={styles.title}>{t('내 보드')}</Text>
        <BoardPage t={t} connected={false} permitNeeded amount={String} actions={[]} back={leave} /></> :
        shown === 'board' ? <><Text accessibilityRole="header" style={styles.title}>{t('내 보드')}</Text>
        <BoardPage t={t} connected setup={<SetupStatus stored={demoLife(0)[1][1].stored} hasEnrollment live={false} open={off} />} amount={units => storeMoney(units)} actions={[]} back={leave}
          spending={{account: demoOwner.address, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: true, sends: true, leftToday: 95_500_000n}} /></> :
        'sentAt' in shown ? <OrderDetail order={shown} amount="4.50 TestUSDC" fresh now={Math.floor(Date.now() / 1000)}
          actions={{answer: off, refresh: off, back: leave}} /> :
        <OrderPayment view={shown} t={t} actions={{findBoard: off, checkBoard: off, approve: off, again: off, openBoard: off, check: off, resend: off, startNew: off,
          cancel: leave, close: leave}} />}
    </ScrollView>;
  }
  const group = (title: string, labels: string[]) => <Section title={t(title)}>
    {labels.map(label => <KitButton key={label} label={t(label)} run={off} disabled variant="row" />)}
  </Section>;
  return <View style={styles.screen} testID="demo-wallet-home">
    <ScrollView contentContainerStyle={styles.content}>
      <View style={styles.banner} accessibilityRole="alert">
        <Text style={styles.bannerTitle}>{t('시연용 화면')}</Text>
        <Text style={styles.bannerText}>{t('실제 보드 확인 없이 만든 예시입니다. 잔액과 주소는 가짜이며 승인·서명·전송은 모두 꺼져 있습니다.')}</Text>
      </View>
      <Brand chip={{label: t('시연용'), tone: 'wait'}} />
      <Text accessibilityRole="header" style={styles.title}>{t('개인 지갑')}</Text>
      <WalletHomeSummary owner={demoOwner} state={state} language="ko" active
        onHide={() => setHidden(value => !value)} onAccounts={off} onReceive={off} inert />
      <BoardCard t={t} state="active" open={() => setShown('board')} limits={{perPayment: 30000000n, daily: 100000000n, leftToday: 92000000n}}
        amount={units => storeMoney(units)} />
      <OpenOrderCard open={off} order={demoOrders(0)[1][1]} />
      <Section title={t('주문 화면 예시')}>{demoOrders(0).map(([label], index) =>
        <KitButton key={label} label={t(label)} variant="row" run={() => setShown(demoOrders(Math.floor(Date.now() / 1000))[index][1])} />)}</Section>
      <Section title={t('보드 화면 예시')}>
        <KitButton label={t('운영자 허가 없음')} variant="row" run={() => setShown('permit')} />
        {demoLife(0).map(([label], index) => <KitButton key={label} label={t(label)} variant="row"
          run={() => setShown(demoLife(Math.floor(Date.now() / 1000))[index][1])} />)}</Section>
      <Section title={t('결제 화면 예시')}>{demoPayments(0).map(([label], index) =>
        <KitButton key={label} label={t(label)} variant="row"
          run={() => setShown(demoPayments(Math.floor(Date.now() / 1000))[index][1])} />)}</Section>
      <View style={styles.pair}>
        <View style={styles.pairItem}><KitButton label={t('다른 체인 받기')} run={off} disabled variant="secondary" /></View>
        <View style={styles.pairItem}><KitButton label={t('다른 체인 보내기')} run={off} disabled variant="secondary" /></View>
      </View>
      {/* Mirrors the verified wallet tab: app connections live in 둘러보기, security in 설정. */}
      {group('자산과 계정', ['6개 체인 · 읽기 전용', 'EVM 수신', '개인 전송', '지급 기록 관리'])}
      <KitButton label={t('시연 화면 닫기')} run={close} variant="primary" />
    </ScrollView>
  </View>;
}

const styles = StyleSheet.create({
  screen: {flex: 1, gap: 12},
  content: {gap: 12, paddingBottom: 24},
  banner: {backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12, gap: 4},
  bannerTitle: {fontFamily: font, fontSize: 15, fontWeight: '800', color: color.amber},
  bannerText: {fontFamily: font, fontSize: 13, lineHeight: 19, color: '#5a3b05'},
  title: {fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink},
  ribbon: {alignSelf: 'flex-start', fontFamily: font, fontSize: 13, fontWeight: '800', color: color.amber, backgroundColor: color.amberBg,
    borderRadius: radius.s, paddingHorizontal: 10, paddingVertical: 4, overflow: 'hidden'},
  pair: {flexDirection: 'row', gap: 10},
  pairItem: {flex: 1},
});
