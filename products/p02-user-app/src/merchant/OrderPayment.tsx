import React, {useEffect, useState} from 'react';
import {ActivityIndicator, StyleSheet, Text, View} from 'react-native';
import {BoardStepPanel} from '../wallet/BoardStepPanel.tsx';
import {Card, KitButton, Row, Section} from '../ui/kit.tsx';
import {color, font, radius, type} from '../ui/theme.ts';
import {AmountLine, PickupTicket} from './paymentParts.tsx';

// States of an order paid with the owner's board into the escrow. CustomerOrders decides the state;
// this file only draws it, so the fixture demo and the tests show the same screens.

export type OrderBlock = 'no-board' | 'wrong-account' | 'unsupported' | 'answered' | 'board-unready' | 'over-limit' | 'over-daily' | 'pin-wait' | 'balance' | 'expired' | 'key-lost';
export type OrderBoard = {perPayment: string; left: string; balance?: string};
export type OrderPaymentView = {store: string; amount: string} & (
  | {kind: 'confirm'; deadline?: number; spent?: boolean; items: {label: string; amount: string}[]; block?: OrderBlock; pinWait?: number;
      board?: OrderBoard; boards?: {handle: string; name: string}[]; searching?: boolean; resumed?: boolean}
  | {kind: 'waiting'; deadline: number}
  | {kind: 'stopped'; reason: 'refused' | 'cancelled' | 'expired' | 'changed' | 'interrupted';
      /** The board will not answer this order again before then. */ until?: number; spent?: boolean;
      /** No signature left the phone, so another order can start now. */ fresh?: boolean}
  | {kind: 'submitting'}
  | {kind: 'unknown'; unsent?: boolean}
  | {kind: 'done'; number: number}
  | {kind: 'failed'}
);
export type OrderPaymentActions = {
  findBoard: () => void; checkBoard: (handle: string) => void; approve: () => void; cancel: () => void; again: () => void;
  openBoard: () => void; check: () => void; resend: () => void; close: () => void; startNew: () => void;
};

const blocks: Record<OrderBlock, {title: string; detail: string; next?: [string, 'openBoard' | 'again' | 'findBoard']}> = {
  'no-board': {title: '연결된 보드가 없습니다', detail: '이 휴대전화에 내 보드를 연결하면 결제할 수 있습니다.', next: ['보드 연결하기', 'openBoard']},
  'wrong-account': {title: '내 보드 계정의 주문이 아닙니다', detail: '이 주문은 다른 계정으로 만들어졌습니다. 키오스크에서 주문을 취소하고 다시 시작하세요.'},
  'board-unready': {title: '보드가 아직 결제할 수 없습니다', detail: '보드 전원과 등록 상태를 확인한 뒤 다시 찾으세요.', next: ['보드 다시 찾기', 'findBoard']},
  'unsupported': {title: '이 매장에서는 아직 결제할 수 없는 주문입니다', detail: '이 매장에서는 아직 구독, 여행 예약, 대여 보드 결제를 받지 않습니다. 빠져나간 돈은 없습니다.'},
  'answered': {title: '보드가 이미 답한 결제입니다', detail: '보드는 같은 결제를 다시 묻지 않습니다. 남은 시간이 지나면 새 결제 내용이 있는지 확인하고, 없으면 키오스크에서 새로 주문하세요.'},
  'over-daily': {title: '오늘 남은 한도를 넘는 금액입니다', detail: '보드가 오늘 더 승인할 수 있는 금액보다 큽니다. 주문을 줄이거나 내일 다시 결제하세요.'},
  'over-limit': {title: '보드 한도를 넘는 금액입니다', detail: '보드는 한 번에 정해진 금액까지만 승인합니다. 키오스크에서 주문을 줄여 주세요.'},
  'pin-wait': {title: 'PIN 입력을 기다려야 합니다', detail: 'PIN을 여러 번 틀려 보드가 잠시 입력을 막았습니다. 시간이 지난 뒤 다시 확인하세요.', next: ['보드 다시 확인', 'findBoard']},
  balance: {title: '보드 계정의 잔액이 부족합니다', detail: '원래 지갑에서 보드 계정으로 입금하거나 키오스크에서 주문을 줄여 주세요.', next: ['보드 계정 보기', 'openBoard']},
  expired: {title: '결제 기한이 지났습니다', detail: '빠져나간 돈은 없습니다. 새 결제 내용이 있는지 확인하고, 없으면 키오스크에서 새로 주문하세요.', next: ['결제 내용 다시 받기', 'again']},
  'key-lost': {title: '이 휴대전화의 주문 확인 키를 찾을 수 없습니다', detail: '끝나지 않은 주문이 이전 키에 묶여 있어 새 키를 만들지 않았습니다. 매장이나 운영자에게 문의하세요.'},
};
const stops: Record<Extract<OrderPaymentView, {kind: 'stopped'}>['reason'], [string, string]> = {
  refused: ['보드에서 거절했습니다', '빠져나간 돈은 없습니다. 다시 결제하려면 새 결제 내용을 받으세요.'],
  cancelled: ['결제를 취소했습니다', '빠져나간 돈은 없습니다. 다시 결제하려면 새 결제 내용을 받으세요.'],
  expired: ['결제 기한이 지났습니다', '빠져나간 돈은 없습니다. 새 결제 내용이 있는지 확인하고, 없으면 키오스크에서 새로 주문하세요.'],
  changed: ['결제 내용이 바뀌었습니다', '빠져나간 돈은 없습니다. 바뀐 내용을 다시 받아 확인하세요.'],
  interrupted: ['보드 연결이 끊겼습니다', '같은 보드에 다시 연결해 결과를 확인하세요.'],
};

/** Seconds left, read once a second. */
function useLeft(deadline: number) {
  const left = () => Math.max(0, deadline - Math.floor(Date.now() / 1000));
  const [seconds, setSeconds] = useState(left);
  useEffect(() => {
    setSeconds(left());
    const timer = setInterval(() => setSeconds(left()), 1000);
    return () => clearInterval(timer);
    // `left` only closes over the deadline.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deadline]);
  return seconds;
}
const clock = (seconds: number) => `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;

function Left({deadline, label, onGreen = false}: {deadline: number; label: string; onGreen?: boolean}) {
  const seconds = useLeft(deadline);
  return <View style={onGreen ? styles.leftGreen : styles.left} accessible accessibilityLabel={`${label} ${Math.floor(seconds / 60)}분 ${seconds % 60}초`}>
    <Text style={[styles.leftLabel, onGreen && styles.onGreen]}>{label}</Text><Text style={[styles.leftTime, onGreen && styles.onGreen]}>{clock(seconds)}</Text>
  </View>;
}

const spentNotice = (amount: string) => `보드가 이미 승인해서 오늘 남은 한도가 ${amount}만큼 줄었습니다.`;

function Stopped({view, actions, busy}: {view: Extract<OrderPaymentView, {kind: 'stopped'}>; actions: OrderPaymentActions; busy: boolean}) {
  const left = useLeft(view.until ?? 0);
  const held = view.until !== undefined && left > 0;
  return <>
    <Notice title={stops[view.reason][0]} tone="stop" detail={held
      ? `빠져나간 돈은 없습니다. 보드는 같은 결제를 다시 묻지 않습니다. ${view.fresh ? '지금 새로 주문하거나, ' : ''}아래 시간이 지나면 새 결제 내용이 있는지 확인하${view.fresh ? '세요.' : '고, 없으면 키오스크에서 새로 주문하세요.'}` : stops[view.reason][1]}>
      {view.spent && <Text style={styles.noticeBody}>{spentNotice(view.amount)}</Text>}
    </Notice>
    {held && <Left deadline={view.until!} label="다시 확인할 때까지" />}
    {view.fresh && <KitButton label="새로 주문하기" run={actions.startNew} disabled={busy} />}
    <KitButton label="결제 내용 다시 받기" run={actions.again} disabled={busy || held} variant={view.fresh ? 'secondary' : 'primary'} />
    <KitButton label="결제 화면 닫기" run={actions.close} disabled={busy} variant="secondary" />
  </>;
}

function Notice({title, detail, tone = 'wait', children}: {title: string; detail: string; tone?: 'wait' | 'stop'; children?: React.ReactNode}) {
  return <View style={[styles.notice, tone === 'stop' && styles.noticeStop]} accessibilityLiveRegion="polite">
    <Text accessibilityRole="header" style={[styles.noticeTitle, tone === 'stop' && styles.stopText]}>{title}</Text>
    <Text style={styles.noticeBody}>{detail}</Text>
    {children}
  </View>;
}

function Confirm({view, actions, busy}: {view: Extract<OrderPaymentView, {kind: 'confirm'}>; actions: OrderPaymentActions; busy: boolean}) {
  const expired = useLeft(view.deadline ?? 0) === 0 && view.deadline !== undefined;
  const reason: OrderBlock | undefined = expired && (!view.block || view.block === 'answered') ? 'expired' : view.block;
  const block = reason && blocks[reason];
  return <>
    <Card tone="green" style={styles.amountCard}>
      <Text style={styles.store}>{view.store}</Text>
      <Text style={styles.amount}>{view.amount}</Text>
      {view.deadline !== undefined && reason !== 'expired' &&
        <Left deadline={view.deadline} label={reason === 'answered' ? '다시 확인할 때까지' : '결제 기한'} onGreen />}
    </Card>
    {!!view.items.length && <Section title="주문 내용"><View style={styles.rows}>
      {view.items.map(item => <Row key={item.label} label={item.label} value={item.amount} />)}
    </View></Section>}
    {view.board && <Section title="보드 확인"><View style={styles.rows}>
      <Row label="한 번에 승인하는 금액" value={view.board.perPayment} />
      <Row label="오늘 남은 한도" value={view.board.left} />
      <Row label="보드 계정 잔액" value={view.board.balance ?? '지금 확인할 수 없음'} />
    </View></Section>}
    {block ? <Notice title={block.title} detail={reason === 'pin-wait' && view.pinWait
      ? `${block.detail} 약 ${Math.ceil(view.pinWait / 60)}분 남았습니다.` : block.detail} tone="stop">
      {view.spent && <Text style={styles.noticeBody}>{spentNotice(view.amount)}</Text>}
      {block.next && <KitButton label={block.next[0]} run={actions[block.next[1]]} disabled={busy} variant="secondary" />}
    </Notice> : <>
      {view.resumed && <Notice title="보드 승인을 이어서 확인합니다"
        detail="이미 보드에서 승인했다면 버튼을 다시 누르지 않아도 결과를 가져옵니다. 두 번 결제되지 않습니다." />}
      {view.board ? <KitButton label="보드로 승인" run={actions.approve} disabled={busy} /> :
        view.boards?.length ? view.boards.map((board, index) => <KitButton key={board.handle} disabled={busy}
          label={`${board.name || '이름 없는 보드'} 확인${view.boards!.length > 1 ? ` · ${index + 1}` : ''}`} run={() => actions.checkBoard(board.handle)} />) :
        <KitButton label={view.searching ? '내 보드를 찾는 중' : '내 보드 확인'} run={actions.findBoard} disabled={busy} />}
    </>}
    <KitButton label="결제 취소" run={actions.cancel} disabled={busy && !view.searching} variant="secondary" />
  </>;
}

const titles: Record<OrderPaymentView['kind'], string> = {
  confirm: '결제 내용을 확인하세요', waiting: '보드에서 승인해 주세요', stopped: '결제하지 않았습니다', submitting: '결제를 보내고 있습니다',
  unknown: '결제 결과를 확인하고 있습니다', done: '주문이 접수되었습니다', failed: '결제되지 않았습니다',
};

export function OrderPayment({view, actions, busy = false, message, t = text => text}: {
  view: OrderPaymentView; actions: OrderPaymentActions; busy?: boolean; message?: string; t?: (text: string) => string;
}) {
  const summary = <Card><Text style={type.label}>{view.store}</Text><AmountLine amount={view.amount} /></Card>;
  return <View style={styles.screen} testID="order-payment">
    <Text accessibilityRole="header" style={type.title}>{titles[view.kind]}</Text>
    {!!message && <Text accessibilityLiveRegion="polite" style={type.body}>{message}</Text>}
    {view.kind === 'confirm' && <Confirm view={view} actions={actions} busy={busy} />}
    {view.kind === 'waiting' && <>
      {summary}
      <Left deadline={view.deadline} label="승인할 수 있는 시간" />
      <BoardStepPanel wait={{flow: 'order', step: 'approve',
        note: '위의 매장과 금액을 확인하고 누르세요. SW1을 누른 뒤 보드 버튼으로 PIN을 입력하면 승인이 끝납니다.'}} t={t} />
      <KitButton label="승인 취소" run={actions.cancel} variant="secondary" />
    </>}
    {view.kind === 'stopped' && <>
      {summary}
      <Stopped view={view} actions={actions} busy={busy} />
    </>}
    {view.kind === 'submitting' && <>
      {summary}
      <View style={styles.progress}><ActivityIndicator color={color.green} accessibilityLabel="결제 확인 중" />
        <Text style={styles.progressText} accessibilityLiveRegion="polite">보드가 승인했습니다. 매장에 결제를 보내고 주문번호를 기다립니다.</Text></View>
      <Text style={type.small}>이 화면을 닫아도 같은 주문을 이어서 확인합니다.</Text>
      <KitButton label="결제 화면 닫기" run={actions.close} variant="secondary" />
    </>}
    {view.kind === 'unknown' && <>
      {summary}
      {view.unsent ? <Notice title="승인한 결제를 아직 보내지 못했습니다" detail="키오스크에 다시 연결하거나 같은 결제를 다시 보내세요. 보드를 다시 누르지 않아도 됩니다." />
        : <Notice title="다시 결제하지 마세요" detail="결제 요청은 보냈지만 결과를 아직 받지 못했습니다. 같은 주문의 결과만 확인하므로 두 번 결제되지 않습니다." />}
      {view.unsent ? <><KitButton label="같은 결제 보내기" run={actions.resend} disabled={busy} />
        <KitButton label="결제 취소" run={actions.cancel} disabled={busy} variant="secondary" /></> : <>
        <KitButton label="결과 다시 확인" run={actions.check} disabled={busy} />
        <KitButton label="같은 결제 다시 보내기" run={actions.resend} disabled={busy} variant="secondary" /></>}
      <KitButton label="결제 화면 닫기" run={actions.close} variant="secondary" />
    </>}
    {view.kind === 'done' && <>
      <PickupTicket number={view.number} store={view.store} />
      <Card><AmountLine amount={view.amount} label="결제 금액" />
        <Text style={type.body}>매장에서 이 번호를 보여 주세요. 준비 상태는 결제 탭에서 볼 수 있습니다.</Text></Card>
      <KitButton label="확인" run={actions.close} />
    </>}
    {view.kind === 'failed' && <>
      {summary}
      <Notice title="빠져나간 돈이 없습니다" detail="결제가 실행되지 않은 것을 확인했습니다. 다시 주문할 수 있습니다." tone="stop" />
      <KitButton label="결제 화면 닫기" run={actions.close} />
    </>}
  </View>;
}

const styles = StyleSheet.create({
  screen: {gap: 14, paddingBottom: 24},
  amountCard: {padding: 20},
  store: {fontFamily: font, fontSize: 17, fontWeight: '800', color: color.mint},
  amount: {fontFamily: font, fontSize: 38, fontWeight: '800', letterSpacing: -1.2, color: '#ffffff'},
  rows: {backgroundColor: color.sheet, paddingHorizontal: 16, paddingVertical: 8},
  left: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12, backgroundColor: color.amberBg,
    borderRadius: radius.m, paddingHorizontal: 14, paddingVertical: 10},
  leftGreen: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginTop: 4},
  onGreen: {color: color.onGreenSoft},
  leftLabel: {fontFamily: font, fontSize: 14, fontWeight: '600', color: '#5a3b05', flexShrink: 1},
  leftTime: {fontFamily: font, fontSize: 18, fontWeight: '800', color: '#5a3b05', fontVariant: ['tabular-nums']},
  notice: {gap: 8, borderRadius: radius.m, padding: 14, backgroundColor: color.amberBg},
  noticeStop: {backgroundColor: color.dangerBg},
  noticeTitle: {fontFamily: font, fontSize: 17, fontWeight: '800', color: '#5a3b05'},
  stopText: {color: color.danger},
  noticeBody: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.ink},
  progress: {flexDirection: 'row', alignItems: 'center', gap: 12, backgroundColor: color.mintSoft, borderRadius: radius.m, padding: 14},
  progressText: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.deep, flex: 1},
});
