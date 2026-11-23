import React from 'react';
import {Pressable, StyleSheet, Text, View} from 'react-native';
import {isFormerBoard, OpenOrderCard} from '../merchant/OpenOrders.tsx';
import type {OpenOrder} from '../merchant/orderRecords.ts';
import {Card, KitButton, Section} from '../ui/kit.tsx';
import {color, font, radius, type} from '../ui/theme.ts';
import {boardSteps} from './boardSteps.ts';
import type {SpendingStatus} from './ownerWire.ts';
import type {OrderPayer} from '../merchant/OpenOrders.tsx';

type Say = (ko: string) => string;

/** First steps of someone whose board is not connected yet. */
export function FirstSteps({t, connect, disabled}: {t: Say; connect: () => void; disabled: boolean}) {
  const steps = [['보드 연결과 PIN 설정', 'SW4를 길게 누르고 앱 안내를 따라요'], ['서명 시험', '보드가 만든 주소를 앱이 검증해요'],
    ['반납받을 주소 등록', '남은 돈이 돌아갈 원래 지갑 주소예요'], ['매장 등록', '나중에 해도 돼요'], ['원래 지갑에서 입금', '쓸 만큼만 넣어 두고 사용해요']];
  return <>
    <Text style={type.body}>{t('보드가 결제 서명을 맡고, 이 앱이 보드의 화면 역할을 합니다. 원래 지갑은 노출되지 않아요.')}</Text>
    <Card style={styles.steps}>{steps.map(([title, detail], index) => <View key={title} style={styles.step}>
      <View style={[styles.number, index === 0 && styles.numberNow]}><Text style={[styles.numberText, index === 0 && styles.numberTextNow]}>{index + 1}</Text></View>
      <View style={styles.words}><Text style={type.label}>{t(title)}</Text><Text style={type.small}>{t(detail)}</Text></View>
    </View>)}</Card>
    <KitButton label={t('보드 연결 시작')} run={connect} disabled={disabled} />
  </>;
}

/** The board's row on the home page; opens 내 보드. */
export function BoardCard({t, state, open}: {t: Say; state: 'active' | 'unchecked' | 'unverified'; open: () => void}) {
  const [title, detail] = {
    active: ['내 보드 연결됨', '보드 상태를 확인했습니다'],
    unchecked: ['내 보드', '보드 상태를 아직 확인하지 않았습니다'],
    unverified: ['보드 연결이 필요합니다', '보드 계정을 확인하면 결제할 수 있어요'],
  }[state];
  return <Pressable accessibilityRole="button" accessibilityLabel={`${t(title)}, ${t(detail)}`} onPress={open}
    style={({pressed}) => [styles.board, pressed && styles.pressed]}>
    <View style={[styles.dot, state === 'active' && styles.dotOn]} />
    <View style={styles.words}><Text style={type.label}>{t(title)}</Text><Text style={type.small}>{t(detail)}</Text></View>
    <Text style={styles.manage}>{t('관리')}</Text>
  </Pressable>;
}

export function HomeOrders({t, orders, open, account}: {t: Say; orders: OpenOrder[]; open: (sale: string) => void; account?: OrderPayer}) {
  if (!orders.length) return null;
  return <View style={styles.orders}>
    <Text accessibilityRole="header" style={type.section}>{t('결제 진행 중')}</Text>
    {orders.map(order => <OpenOrderCard key={order.sale} order={order} former={isFormerBoard(order, account)} open={() => open(order.sale)} />)}
  </View>;
}

// Every pattern the spending firmware plays (p01 app/led-patterns.json): all four LEDs act as one lamp.
const signals: [string, string][] = [
  ['대기', '꺼짐'], ['연결 창 열림', '2초마다 짧게 한 번'], ['승인 대기', boardSteps.approve.led!], ['누름 접수', '0.6초 켜짐'],
  ['PIN 입력 시작', '길게 두 번'], ['PIN 숫자 올림', '짧게 한 번'], ['PIN 다음 자리', '0.5초 한 번'],
  ['서명 완료', '느리게 깜박임(0.5초 간격, 최대 30초)'], ['거절, 실패 또는 초기화 취소', '짧게 세 번'],
  ['PIN 입력 대기 중', '아주 짧게 다섯 번'], ['초기화 확인 대기', '아주 빠르게 깜박임(0.08초 간격)'], ['초기화 완료', '1.5초씩 두 번'],
];

/** 내 보드: state, limits, setup, LED signals and the entries for deposit, return and reset. */
export function BoardPage({t, connected, spending, permitNeeded = false, setup, amount, actions, back}: {
  t: Say; connected: boolean; spending?: SpendingStatus; permitNeeded?: boolean; setup?: React.ReactNode; amount: (units: bigint) => string; actions: {label: string; detail?: string; run: () => void}[]; back: () => void;
}) {
  return <>
    <Card>
      <Text style={type.section}>{t(permitNeeded ? '운영자 허가가 필요합니다' : connected ? '보드 상태 확인됨' : '보드 상태 확인 전')}</Text>
      {permitNeeded && <Text style={type.body}>{t('보드가 연결을 끝내지 않았습니다. 보드를 켠 뒤 운영자가 허가를 넣지 않았거나 허가가 끝난 상태입니다. 다른 보드를 골랐을 때도 연결되지 않습니다.')}</Text>}
      {spending ? <>
        <Text style={type.body}>{`${t('한 번에')} ${amount(spending.perPayment)} · ${t('하루에')} ${amount(spending.daily)}`}</Text>
        <Text style={type.body}>{`${t('오늘 남은 한도')} ${amount(spending.leftToday)}`}</Text>
        {spending.pinWait > 0 && <Text style={styles.wait}>{`${t('PIN 입력 대기 중')} · ${Math.ceil(spending.pinWait / 60)}${t('분')}`}</Text>}
        {spending.orderOpen && <Text style={styles.wait}>{t('방금 승인한 결제의 기한이 남아 있습니다. 반납과 초기화는 최대 4분 뒤에 할 수 있습니다.')}</Text>}
        <Text style={type.body}>{`${t('반납받을 주소')} ${t(spending.returnAddress ? '저장됨' : '없음')}`}</Text>
      </> : <Text style={type.small}>{t('보드를 연결하면 한도와 PIN 상태를 볼 수 있습니다.')}</Text>}
    </Card>
    {setup}
    <Section title={t('LED 신호 보는 법')}>{signals.map(([meaning, pattern]) => <View key={meaning} style={styles.signal} accessible>
      <Text style={styles.meaning}>{t(meaning)}</Text><Text style={styles.pattern}>{t(pattern)}</Text>
    </View>)}</Section>
    {!!actions.length && <Section title={t('보드 관리')}>{actions.map(action =>
      <KitButton key={action.label} label={action.label} detail={action.detail} run={action.run} variant="row" />)}</Section>}
    <KitButton label={t('돌아가기')} run={back} variant="secondary" />
  </>;
}

const styles = StyleSheet.create({
  steps: {gap: 0, paddingVertical: 4},
  step: {flexDirection: 'row', alignItems: 'center', gap: 14, paddingVertical: 12},
  number: {minWidth: 30, height: 30, borderRadius: 15, backgroundColor: color.mintSoft, alignItems: 'center', justifyContent: 'center'},
  numberNow: {backgroundColor: color.green},
  numberText: {fontFamily: font, fontSize: 14, fontWeight: '800', color: color.deep}, numberTextNow: {color: color.onGreen},
  words: {flex: 1, gap: 2},
  board: {flexDirection: 'row', alignItems: 'center', gap: 12, padding: 16, borderRadius: radius.l, borderWidth: 1, borderColor: color.line, backgroundColor: color.sheet},
  pressed: {opacity: 0.82},
  dot: {width: 10, height: 10, borderRadius: 5, backgroundColor: color.line}, dotOn: {backgroundColor: '#2f8f5b'},
  manage: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.green},
  orders: {gap: 10},
  wait: {fontFamily: font, fontSize: 15, fontWeight: '700', color: color.amber},
  signal: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12, minHeight: 48, paddingHorizontal: 16, paddingVertical: 10, backgroundColor: color.sheet},
  meaning: {fontFamily: font, fontSize: 15, fontWeight: '700', color: color.ink, flexShrink: 1},
  pattern: {fontFamily: font, fontSize: 14, color: color.mute, flexShrink: 1, textAlign: 'right'},
});
