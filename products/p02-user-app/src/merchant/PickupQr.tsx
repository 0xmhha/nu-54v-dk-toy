import React, {useEffect, useMemo, useRef, useState} from 'react';
import {AppState, Image, StyleSheet, Text, View} from 'react-native';
import {Card, KitButton} from '../ui/kit.tsx';
import {color, font, radius, type} from '../ui/theme.ts';
import {receiveQr} from '../wallet/receivePresentation.ts';
import {shortAccount, type IssuedPickup} from './customerEntrance.ts';
import type {OpenOrder} from './orderRecords.ts';

/** Seconds before the end at which a new proof is asked for, so the next QR is ready when this one ends. */
const renewBefore = 5;
/** QRs made one after another without a touch; then the customer asks for the next one. Provisional (owner decision). */
export const automaticQrs = 10;
const fallbackLifetime = 60;
const systemNow = () => Date.now();

type Screen = {kind: 'checking'} | {kind: 'ended'} | IssuedPickup | {kind: 'no-entrance'};
type Shown = {qr: string; until: number};

const clock = (seconds: number) => `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;

/**
 * The pickup QR of a ready order. Each QR comes from a new account proof of the paying board and lives about a
 * minute; a new one is asked for shortly before the end while the screen is in front.
 */
export function PickupQrScreen({order, issue, back, contact, now = systemNow}: {
  order: OpenOrder;
  /** One new QR, or what stands in the way. Undefined when this app has no entrance address. */
  issue?: () => Promise<IssuedPickup>;
  back: () => void;
  /** Opens a way to reach the store, when the app has one. */
  contact?: () => void;
  now?: () => number;
}) {
  const [view, setView] = useState<Screen>(issue ? {kind: 'checking'} : {kind: 'no-entrance'});
  const [shown, setShown] = useState<Shown>();
  const [tick, setTick] = useState(() => now());
  const live = useRef(true), asking = useRef(false), made = useRef(0), shownRef = useRef<Shown>(undefined);
  shownRef.current = shown;

  const ask = async (byHand: boolean) => {
    if (!issue || asking.current) return;
    if (byHand) made.current = 0;
    else if (made.current >= automaticQrs) { setView({kind: 'ended'}); return; }
    asking.current = true;
    if (!shownRef.current || shownRef.current.until <= now()) setView({kind: 'checking'});
    try {
      const next = await issue().catch((): IssuedPickup => ({kind: 'unavailable'}));
      if (!live.current) return;
      if (next.kind === 'qr') {
        made.current++;
        // The phone's clock may differ from the server's; a lifetime that makes no sense is taken as the usual minute.
        const left = next.expiresAt - Math.floor(now() / 1000);
        const lifetime = left >= renewBefore * 2 && left <= fallbackLifetime ? left : fallbackLifetime;
        setShown({qr: next.qr, until: now() + lifetime * 1000});
        setTick(now());
      } else setShown(undefined);
      setView(next);
    } finally {
      asking.current = false;
    }
  };

  useEffect(() => {
    live.current = true;
    if (issue) ask(true).catch(() => {});
    // A QR is not renewed in the background; coming back asks for a new one if this one has ended.
    const state = AppState.addEventListener('change', next => {
      if (next === 'active' && shownRef.current && shownRef.current.until <= now()) ask(false).catch(() => {});
    });
    return () => { live.current = false; state.remove(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (view.kind !== 'qr') return;
    const timer = setInterval(() => setTick(now()), 1000);
    return () => clearInterval(timer);
  }, [view.kind, now]);

  const left = shown ? Math.max(0, Math.ceil((shown.until - tick) / 1000)) : 0;
  useEffect(() => {
    if (view.kind !== 'qr' || !shown || left > renewBefore || AppState.currentState === 'background') return;
    // After the automatic ones the last QR runs out before the customer is asked to get the next.
    if (made.current < automaticQrs) ask(false).catch(() => {});
    else if (left === 0) setView({kind: 'ended'});
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view.kind, left <= renewBefore, left === 0]);

  // The countdown redraws the page every second; the QR is drawn only when it changes.
  const qrText = shown?.qr;
  const image = useMemo(() => qrText ? receiveQr(qrText) : '', [qrText]);
  const title = `${order.store ?? '매장 주문'}${order.pickupNo === undefined ? '' : ` · ${order.pickupNo}번`}`;
  const items = order.items?.map(item => item.name).join(' · ');
  const again = <KitButton label="다시 확인하기" run={() => { ask(true).catch(() => {}); }} />;
  const leave = <KitButton label="주문으로 돌아가기" run={back} variant="secondary" />;

  if (view.kind === 'qr' || (view.kind === 'checking' && shown && left > 0)) return <View style={styles.page} testID="pickup-qr">
    <Text accessibilityRole="header" style={type.title}>{title}</Text>
    <Text style={type.body}>직원에게 이 QR을 보여 주세요. 직원이 찍으면 전달이 기록됩니다.</Text>
    <View style={styles.qrBox}>
      {left > 0 ? <Image accessibilityLabel="수령 QR" source={{uri: image}} style={styles.qr} />
        : <Text accessibilityLiveRegion="polite" style={type.body}>새 QR을 받고 있습니다.</Text>}
    </View>
    <View style={styles.timer} accessible accessibilityLabel={`이 QR을 쓸 수 있는 시간 ${left}초`}>
      <Text style={type.small}>이 QR을 쓸 수 있는 시간</Text>
      <Text style={styles.time}>{clock(left)}</Text>
      <Text style={type.small}>지나면 결제한 보드로 다시 확인해 새 QR로 바꿉니다</Text>
    </View>
    <Card>
      {!!items && <View style={styles.row}><Text style={type.small}>주문</Text><Text style={[type.body, styles.value]}>{items}</Text></View>}
      <View style={styles.row}><Text style={type.small}>확인한 보드</Text><Text style={[type.body, styles.value]}>이 주문을 결제한 보드</Text></View>
    </Card>
    <Text style={type.small}>QR에는 한 번만 쓰는 값만 들어 있습니다. 주문 내용과 금액, 내 계정은 들어 있지 않습니다. 화면을 다른 사람에게 보내면 그 사람이 받아 갈 수 있습니다.</Text>
    {leave}
  </View>;

  if (view.kind === 'checking') return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{title}</Text>
    <Text accessibilityLiveRegion="polite" style={type.body}>결제한 보드로 확인하고 있습니다. 보드를 켜고 휴대전화 가까이 두세요.</Text>
    {leave}
  </View>;

  if (view.kind === 'no-board') return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>결제한 보드가 필요합니다</Text>
    <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>
      <Text style={styles.strong}>보드를 찾지 못했습니다</Text>{'\n'}수령 QR은 이 주문을 결제한 보드로 확인한 뒤에만 나옵니다. 보드를 켜고 휴대전화 가까이 두세요.</Text></View>
    <View style={styles.ok}><Text style={styles.okText}>주문은 그대로 있습니다. 준비된 음료는 매장이 보관합니다.</Text></View>
    {again}
    {contact && <KitButton label="매장에 연락" run={contact} variant="secondary" />}
    {leave}
  </View>;

  if (view.kind === 'other-board') return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>결제한 보드가 아닙니다</Text>
    <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>
      <Text style={styles.strong}>연결된 보드의 계정이 이 주문의 결제 계정과 다릅니다</Text>{'\n'}다른 보드로는 이 주문을 받을 수 없습니다. 결제한 보드를 연결하세요.</Text></View>
    <Card>
      <View style={styles.row}><Text style={type.small}>이 주문의 결제 계정</Text><Text style={[type.body, styles.value]}>{shortAccount(view.payer)}</Text></View>
      <View style={styles.row}><Text style={type.small}>연결된 보드의 계정</Text><Text style={[type.body, styles.value]}>{shortAccount(view.board)}</Text></View>
    </Card>
    <KitButton label="결제한 보드로 다시 확인" run={() => { ask(true).catch(() => {}); }} />
    {leave}
  </View>;

  const [head, body] = view.kind === 'not-ready'
    ? ['아직 수령 QR을 받을 수 없습니다', '매장이 준비 완료를 기록한 주문에만 수령 QR이 나옵니다. 이미 전달이 기록되었다면 주문 상태에서 확인하세요.']
    : view.kind === 'busy' ? ['요청이 많습니다', '잠시 뒤 다시 확인해 주세요. 주문은 그대로 있습니다.']
    : view.kind === 'ended' ? ['QR을 여러 번 바꿨습니다', '아직 매장에 있다면 새 QR을 받으세요.']
    : view.kind === 'no-entrance' ? ['수령 QR을 받을 수 없습니다', '이 앱에는 수령 QR을 받을 매장 서버 주소가 아직 없습니다. 매장에 문의하세요.']
    : ['수령 QR을 받지 못했습니다', '매장 서버에 닿지 못했습니다. 잠시 뒤 다시 확인하세요. 주문은 그대로 있습니다.'];
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{head}</Text>
    <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>{body}</Text></View>
    {view.kind === 'ended' ? <KitButton label="새 QR 받기" run={() => { ask(true).catch(() => {}); }} /> : view.kind !== 'no-entrance' && again}
    {leave}
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14, paddingBottom: 24},
  qrBox: {alignSelf: 'center', width: 248, height: 248, borderRadius: 20, borderWidth: 2, borderColor: color.ink,
    backgroundColor: '#ffffff', alignItems: 'center', justifyContent: 'center'},
  qr: {width: 228, height: 228},
  timer: {alignItems: 'center', gap: 4, padding: 14, borderRadius: radius.l, backgroundColor: color.sheet, borderWidth: 1, borderColor: color.line},
  time: {fontFamily: font, fontSize: 40, lineHeight: 46, fontWeight: '800', color: color.ink, fontVariant: ['tabular-nums']},
  row: {flexDirection: 'row', justifyContent: 'space-between', gap: 12, paddingVertical: 6},
  value: {flexShrink: 1, textAlign: 'right'},
  warn: {padding: 14, borderRadius: radius.l, backgroundColor: color.amberBg},
  warnText: {fontFamily: font, fontSize: 15, lineHeight: 22, color: '#5a3b05'},
  strong: {fontSize: 17, fontWeight: '700'},
  ok: {padding: 14, borderRadius: radius.l, backgroundColor: color.mintSoft},
  okText: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.deep},
});
