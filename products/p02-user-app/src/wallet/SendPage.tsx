import React, {useCallback, useEffect, useRef, useState} from 'react';
import {StyleSheet, Text, TextInput, View} from 'react-native';
import {bytesToHex, fromBase64, fromBoard, type PaymentToken, type WalletSends} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import ShopQr from '../specs/NativeShopQr.ts';
import {walletId} from '../merchant/customerClient.ts';
import {routeWords} from '../merchant/orderRoute.ts';
import {AmountLine} from '../merchant/paymentParts.tsx';
import {Card, KitButton, Row} from '../ui/kit.tsx';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import {Chooser, credential, Notice, shownToken, signingToken, tokenFailures, useBalance, type BoardLifeProps} from './BoardLife.tsx';
import {atOtherStore, boardFailure, serverFailure} from './boardReturn.ts';
import {newSend, renewSend, sendRecord, sendStale, settleSend, submitSend, verifySend, type SendRecord} from './boardSend.ts';
import {parseHumanAmount} from './evmRegistry.ts';
import {normalizeEvmAddress} from './evmTransfer.ts';
import {testedBoard} from './profile.ts';
import {defaultReadState, readContact} from './readProfiles.ts';
import {assetMoney, storeMoney, type Shown} from './tokens.ts';

const now = () => Math.floor(Date.now() / 1000);
const pinNote = '위 금액과 받는 주소를 확인한 뒤 SW1을 짧게 누르고 보드 버튼으로 PIN을 입력하세요. 거절하려면 SW2를 누르세요.';

/** Dollars are typed in cents, as the screens show them; a token with fewer decimals in all of them. */
const sendDecimals = (token: PaymentToken) => Math.min(2, token.decimals);
const decimalWords = (decimals: number) => ['없음', '한', '두', '세', '네', '다섯', '여섯'][decimals] ?? String(decimals);

/** What the server's refusal of a transfer means, in the words of this build's route. The rest is worded as for a return. */
export function sendFailure(error: unknown): string {
  return routeWords(({
    UNSUPPORTED_CAPABILITY: '이 매장은 보내기를 중계하지 않습니다.',
    CHECKOUT_CONFLICT: '반납을 진행하고 있어 보낼 수 없습니다.',
    REPLAY_CONFLICT: '이미 보내는 중인 거래가 있습니다. 그 결과를 기다리세요.',
    BUSY: '오늘 보낼 수 있는 횟수를 다 썼거나 서버가 바쁩니다. 나중에 다시 시도하세요.',
    NO_CREDENTIAL: '먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.',
    ...tokenFailures,
    SEND_EXPIRED: '보드가 승인한 시간이 지났습니다. 처음부터 다시 시도하세요. 빠져나간 돈은 없습니다.',
  } as Record<string, string>)[error instanceof Error ? error.message : ''] ?? serverFailure(error));
}
const boardWords = (error: unknown) => ({
  BOARD_NO_SEND: '이 보드의 펌웨어는 아직 보내기를 지원하지 않습니다. 빠져나간 돈은 없습니다.',
  SEND_LIMIT: '보드 한도를 넘는 금액입니다. 한도 안에서 다시 입력하세요.',
  SEND_SIGNATURE: '보드의 서명을 확인할 수 없어 보내지 않았습니다.',
  RETURN_NOT_TAKEN: '보드가 요청을 받지 않았습니다. 보드의 시각이 휴대전화와 몇 분 넘게 다르면 이렇게 됩니다. 운영자에게 새 허가를 받은 뒤 다시 시도하세요. 빠져나간 돈은 없습니다.',
} as Record<string, string>)[error instanceof Error ? error.message : ''] ?? boardFailure(error);

/** An address as it is pasted or scanned: bare, or an ethereum: link for this network without a token or amount. */
export function readRecipient(text: string) {
  const match = /^(?:ethereum:)?(0x[0-9a-fA-F]{40})(?:@(8283))?$/.exec(text.trim());
  if (!match) throw new Error('EVM_RECIPIENT');
  return normalizeEvmAddress(match[1]);
}

/** 보내기: the store token to any address. The board signs it, the store relays it and pays the gas. */
export function SendPage(props: BoardLifeProps) {
  // Whole won on the screen: what is held or left is cut off, what is sent is rounded up. The board signs `units`.
  const {store} = props, money = (units: string | bigint, shown: Shown = 'balance') => storeMoney(units, store?.token, 'ko', shown);
  const {stored, work, withBoard, say} = props;
  const {verified} = stored, board = testedBoard(stored);
  const {balance, read} = useBalance(props);
  const [record, setRecord] = useState<SendRecord | null | undefined>(props.fixture ? props.fixture.send ?? null : undefined);
  const [held, setHeld] = useState<WalletSends>(), [problem, setProblem] = useState('');
  const [to, setTo] = useState(''), [amount, setAmount] = useState(''), [label, setLabel] = useState('');
  const [review, setReview] = useState<{to: string; units: bigint}>();
  const [clock, setClock] = useState(now());
  const keep = useCallback(async (next?: SendRecord) => { await sendRecord(Http!).save(next); setRecord(next ?? null); }, []);
  const list = useCallback(async () => {
    const {client} = await credential();
    setHeld(await client.walletSends());
    setProblem('');
  }, []);
  useEffect(() => {
    let live = true;
    if (props.fixture) return;
    if (!Http) { setRecord(null); return; }
    sendRecord(Http).load().then(value => { if (live) setRecord(value ?? null); }).catch(() => { if (live) say('보내기 기록을 읽을 수 없습니다. 지원을 요청하세요.'); });
    list().catch(error => { if (live) setProblem(sendFailure(error)); });
    return () => { live = false; };
  }, [say, props.fixture, list]);
  const check = useCallback(async () => {
    const {client} = await credential();
    const next = await settleSend(Http!, client, now);
    setRecord(next ?? null);
    if (next?.stage !== 'sent') { read().catch(() => {}); list().catch(() => {}); }
  }, [read, list]);
  const waiting = record?.stage === 'sent' && !props.fixture, counting = record?.stage === 'signing' || record?.stage === 'signed';
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => { check().catch(() => {}); }, 10_000);
    return () => clearInterval(timer);
  }, [waiting, check]);
  useEffect(() => {
    if (!counting) return;
    const timer = setInterval(() => setClock(now()), 5000);
    return () => clearInterval(timer);
  }, [counting]);
  const latest = useRef(review);
  latest.current = review;
  if (record === undefined) return <Text style={type.body}>보내기 상태를 확인하고 있습니다.</Text>;

  if (record) {
    const contact = stored.walletRead?.contacts.find(item => item.id === 'stable' && item.address === record.to);
    const summary = <Card><AmountLine amount={assetMoney(record.amount, record.token, store, 'ko', 'charge')} label="보내는 금액" />
      <Row label="네트워크" value="StableNet 테스트넷" /><Row label="수수료" value="매장이 냅니다" />
      <Text style={type.small}>{`받는 주소${contact ? ` · ${contact.name}` : ''}`}</Text><Text selectable style={styles.address}>{record.to}</Text></Card>;
    const drop = (text: string) => work(async () => { await keep(undefined); setReview(undefined); say(text); }, () => '보내기 기록을 바꾸지 못했습니다.');
    if (record.stage === 'final') return <>{summary}<Notice tone="done" title="보냈습니다" body="거래가 확정되었습니다." />
      <KitButton label="확인" run={() => { setTo(''); setAmount(''); setLabel(''); drop(''); }} disabled={props.busy} /></>;
    if (record.stage === 'failed') return <>{summary}<Notice tone="stop" title="보내지 않았습니다" body="거래가 실행되지 않은 것을 확인했습니다. 빠져나간 돈은 없습니다." />
      <KitButton label="확인" run={() => drop('')} disabled={props.busy} /></>;
    const other = !verified || verified.address !== record.payer || String(verified.generation) !== record.generation;
    if (record.stage === 'sent' && atOtherStore(record, store?.branch)) return <>{summary}
      <Notice tone="stop" title="결과 확인 안 됨" body={tokenFailures.SENT_AT_OTHER_STORE} /></>;
    if (record.stage === 'sent') return <>{summary}
      <Notice title="보낸 거래를 확인하고 있습니다" body="확정될 때까지 기다립니다. 두 번 보내지 않습니다. 이 화면을 닫아도 결과는 다시 확인합니다." />
      <KitButton label="지금 확인" run={() => work(check, sendFailure)} disabled={props.busy} />
      <KitButton label="같은 거래 다시 보내기" variant="secondary" disabled={props.busy || record.validBefore <= now()} run={() => work(async () => {
        const {client} = await credential();
        await submitSend(client, record, async next => keep(next), now(), (await signingToken(record.token)).name);
        await check();
      }, sendFailure)} /></>;
    if (other || sendStale(record, clock)) return <>{summary}<Notice body={other ? '다른 보드 계정의 보내기입니다. 보내지 않았고 빠져나간 돈도 없습니다.' : '승인한 시간이 지났습니다. 보내지 않았고 빠져나간 돈도 없습니다.'} />
      <KitButton label="처음부터 다시" run={() => drop('')} disabled={props.busy} /></>;
    if (record.stage === 'signed') return <>{summary}
      <Text style={type.body}>보드가 승인한 거래를 아직 보내지 않았습니다.</Text>
      <KitButton label="보내기" disabled={props.busy} run={() => work(async () => {
        const {client} = await credential();
        setRecord(await submitSend(client, record, async next => keep(next), now(), (await signingToken(record.token)).name));
        await check();
      }, sendFailure)} />
      {record.sentAt === undefined && <KitButton label="보내기 취소" run={() => drop('보내지 않았습니다.')} disabled={props.busy} variant="secondary" />}</>;
    return <>{summary}
      <Text style={type.body}>보드에는 화면이 없습니다. 위 금액과 주소를 확인하고 보드에서 승인하세요.</Text>
      <Chooser props={props} label="보드로 승인" run={handle => work(async () => {
        const token = await shownToken(record.token, store), {name} = token;
        const signed = await withBoard(handle, async session => {
          const asked = renewSend(record, now());
          await keep(asked);
          const signature = await session.signSend(BigInt(asked.generation), {...asked, amount: BigInt(asked.amount)},
            () => props.wait({flow: 'send', step: 'approve', note: pinNote}), name, token);
          const next = verifySend(asked, signature, name);
          await keep(next);
          return next;
        }).catch(error => { throw new Error(boardWords(error)); });
        // The signature is written down and lives for minutes only, so it is sent at once.
        const {client} = await credential();
        await submitSend(client, signed, async next => keep(next), now(), (await signingToken(record.token)).name);
        await check();
      }, error => error instanceof Error && /[가-힣]/.test(error.message) ? error.message : sendFailure(error))} />
      <KitButton label="보내기 취소" run={() => drop('보내지 않았습니다.')} disabled={props.busy} variant="secondary" /></>;
  }

  if (!verified || !board) return <Notice body="먼저 보드 연결과 서명 시험을 마치세요." />;
  if (problem) return <><Notice body={problem} /><KitButton label="다시 확인" run={() => work(list, sendFailure)} disabled={props.busy} variant="secondary" /></>;
  if (!props.fixture && !held) return <Text style={type.body}>보내기를 확인하고 있습니다.</Text>;
  if (props.storeRead === 'loading') return <Text style={type.body}>{routeWords('매장 등록을 확인하고 있습니다.')}</Text>;
  if (!store) return <><Notice body={routeWords('매장 등록을 확인하지 못했습니다. 등록했다면 다시 확인하고, 아니면 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.')} />
    <KitButton label="등록 다시 확인" run={() => props.readStore?.()} variant="secondary" disabled={props.busy} /></>;
  if (props.spending && !props.spending.sends) return <Notice title="아직 보낼 수 없습니다" body="연결한 보드의 펌웨어가 보내기를 지원하지 않습니다. 남은 돈은 반납으로 원래 지갑에 보낼 수 있습니다.">
    <KitButton label="반납 보기" run={() => props.go('return')} variant="secondary" /></Notice>;
  const contacts = (stored.walletRead?.contacts ?? []).filter(item => item.id === 'stable');
  const inspect = () => {
    try {
      const target = readRecipient(to);
      if (target === board.account) { say('보드 계정 자신에게는 보낼 수 없습니다.'); return; }
      const typed = sendDecimals(store.token), units = parseHumanAmount(amount.trim(), typed) * 10n ** BigInt(store.token.decimals - typed);
      if (!balance) { say('보드 계정의 잔액을 확인하지 못했습니다. 잠시 뒤 다시 시도하세요.'); return; }
      if (units > balance.latest || units > balance.units) { say(`잔액보다 큰 금액입니다. 확정된 잔액은 ${money(balance.units)}입니다.`); return; }
      if (props.spending && (units > fromBoard(props.spending.perPayment, store.token) || units > fromBoard(props.spending.leftToday, store.token))) { say('보드 한도를 넘는 금액입니다. 내 보드에서 한도를 확인하세요.'); return; }
      say('');
      setReview({to: target, units});
    } catch (error) {
      say(error instanceof Error && error.message.startsWith('EVM_RECIPIENT') ? '받는 주소가 맞지 않습니다. 다시 복사하거나 QR을 읽으세요.' : '금액이 맞지 않습니다. 소수점 아래 ' + decimalWords(sendDecimals(store.token)) + ' 자리까지 입력할 수 있습니다.');
    }
  };
  const scan = async () => {
    if (!ShopQr) return;
    try {
      const raw = await ShopQr.scan();
      if (raw) { setTo(readRecipient(raw)); say(''); }
    } catch { say('QR에서 받는 주소를 읽지 못했습니다. 주소를 붙여 넣으세요.'); }
  };
  if (review) {
    const known = contacts.find(item => item.address === review.to), before = held?.sends.some(item => item.to === review.to && item.state === 'final');
    return <>
      <Card><AmountLine amount={money(review.units, 'charge')} label="보내는 금액" />
        <Row label="네트워크" value="StableNet 테스트넷" /><Row label="수수료" value="매장이 냅니다" />
        <Row label="보낸 뒤 남는 금액" value={balance ? money(balance.units - review.units) : '확인 안 됨'} />
        <Text style={type.small}>{`받는 주소${known ? ` · ${known.name}` : ''}`}</Text><Text selectable style={styles.address}>{review.to}</Text></Card>
      {!known && !before && <Notice title="처음 보내는 주소입니다" body="앞 여섯 글자와 뒤 네 글자를 받는 사람과 확인하세요. 보낸 돈은 되돌릴 수 없습니다." />}
      {!known && <><TextInput accessibilityLabel="주소록 이름" placeholder="주소록 이름 (선택)" placeholderTextColor={inputPlaceholder} value={label} onChangeText={setLabel}
        style={styles.input} maxLength={32} editable={!props.busy} />
        {!!label.trim() && <KitButton label="주소록에 저장" variant="secondary" disabled={props.busy} run={() => work(async () => {
          await props.commit(head => {
            const state = head.walletRead ?? defaultReadState;
            const others = state.contacts.filter(item => item.id !== 'stable' || item.address !== review.to);
            if (others.length >= 30) throw new Error('CONTACTS_FULL');
            return {...head, walletRead: {...state, contacts: [...others, readContact('stable', label, review.to)]}};
          });
          say('주소록에 저장했습니다.');
        }, error => error instanceof Error && error.message === 'CONTACTS_FULL' ? '주소록에 30개까지 저장할 수 있습니다.' : '주소록에 저장하지 못했습니다.')} />}</>}
      <KitButton label="이 내용으로 보내기" disabled={props.busy} run={() => work(async () => {
        const {client, policy} = await credential();
        const payer = await client.enrolledPayer();
        if (payer?.payer !== verified.address || payer.generation !== String(verified.generation)) throw new Error('MISMATCHED_EVIDENCE');
        const fresh = await client.walletSends();
        setHeld(fresh);
        if (fresh.remaining < 1) throw new Error('BUSY');
        const chosen = latest.current!;
        await shownToken(policy.asset, store);
        const nonce = bytesToHex(fromBase64(await Http!.randomBytes(32)), false);
        await keep(newSend({payer: verified.address, generation: String(verified.generation), device: board.device, token: policy.asset.toLowerCase(), to: chosen.to},
          chosen.units, nonce, await walletId(), now()));
      }, sendFailure)} />
      <KitButton label="고치기" run={() => setReview(undefined)} variant="secondary" disabled={props.busy} />
    </>;
  }
  return <>
    <Card><Row label="토큰" value={`${store.token.label} · StableNet 테스트넷`} /><Row label="가진 금액" value={balance === undefined ? '확인하는 중' : balance ? money(balance.units) : '확인 안 됨'} />
      {props.spending && <><Row label="한 번에" value={money(fromBoard(props.spending.perPayment, store.token))} /><Row label="오늘 남은 한도" value={money(fromBoard(props.spending.leftToday, store.token))} /></>}
      {held && <Row label="오늘 남은 횟수" value={`${held.remaining} / ${held.perDay}`} />}</Card>
    {balance === null && <KitButton label="잔액 다시 확인" run={() => work(async () => { await read(); }, () => '잔액을 확인하지 못했습니다.')} variant="secondary" disabled={props.busy} />}
    <Text style={styles.label}>받는 주소</Text>
    <TextInput accessibilityLabel="받는 주소" placeholder="0x" placeholderTextColor={inputPlaceholder} value={to} onChangeText={setTo} style={styles.input}
      maxLength={80} autoCapitalize="none" autoCorrect={false} editable={!props.busy} />
    {ShopQr && <KitButton label="QR로 받는 주소 읽기" run={() => { scan().catch(() => {}); }} variant="secondary" disabled={props.busy} />}
    {!!contacts.length && <View style={styles.list}>{contacts.map(item => <KitButton key={item.address} label={item.name} detail={`${item.address.slice(0, 8)}…${item.address.slice(-4)}`}
      variant="row" disabled={props.busy} run={() => { setTo(item.address); say(''); }} />)}</View>}
    <Text style={styles.label}>금액</Text>
    <TextInput accessibilityLabel="금액" placeholder="0.00" placeholderTextColor={inputPlaceholder} value={amount} onChangeText={setAmount} style={styles.input}
      maxLength={24} keyboardType="decimal-pad" editable={!props.busy} />
    <KitButton label="다음" run={inspect} disabled={props.busy || !to.trim() || !amount.trim() || (!!held && held.remaining < 1)} />
    {!!held && held.remaining < 1 && <Notice body="오늘 보낼 수 있는 횟수를 다 썼습니다. 24시간이 지나면 다시 보낼 수 있습니다." />}
  </>;
}

const styles = StyleSheet.create({
  label: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.mute},
  address: {fontFamily: 'monospace', fontSize: 14, lineHeight: 20, color: color.ink, backgroundColor: color.ground, borderRadius: 8, padding: 10},
  input: {fontFamily: font, fontSize: 15, borderWidth: 1, borderColor: '#8fa497', borderRadius: radius.m, padding: 12, minHeight: 50, color: color.ink, backgroundColor: color.sheet},
  list: {borderRadius: radius.m, overflow: 'hidden', borderWidth: 1, borderColor: color.line, gap: 1, backgroundColor: color.line},
});
