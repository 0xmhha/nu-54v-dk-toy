import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Linking, StyleSheet, Text, View} from 'react-native';
import {stableNetCoin, type FaucetStatus, type FaucetToken} from '@nu54/protocol';
import {walletId} from '../merchant/customerClient.ts';
import {routeWords} from '../merchant/orderRoute.ts';
import {Card, Chip, KitButton, Row} from '../ui/kit.tsx';
import {color, font, radius, type} from '../ui/theme.ts';
import {credential} from './BoardLife.tsx';
import {tokenText} from './tokens.ts';

type Say = (ko: string) => string;
const reason = (error: unknown) => error instanceof Error ? error.message : '';

/** What the server's answer to a request for test tokens means to the owner, in the words of this build's route. */
export const faucetFailure = (error: unknown) => routeWords(faucetAnswer(error));
function faucetAnswer(error: unknown) {
  switch (reason(error)) {
    case 'NO_CREDENTIAL': return '먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.';
    case 'UNSUPPORTED_CAPABILITY': return '이 매장에서는 시험 토큰을 주지 않습니다.';
    case 'NOT_AUTHORIZED': return '이 휴대전화의 주문 등록으로는 받을 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.';
    case 'MISMATCHED_EVIDENCE': return '이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.';
    case 'REPLAY_CONFLICT': return '이미 보내는 중인 시험 토큰이 있습니다.';
    case 'CHECKOUT_CONFLICT': return '반납을 진행하는 동안에는 받을 수 없습니다.';
    case 'BUSY': return '오늘 받을 수 있는 만큼 받았거나 서버가 바쁩니다. 나중에 다시 시도하세요.';
    case 'OUT_OF_STOCK': return '줄 수 있는 시험 토큰이 모자랍니다. 운영자에게 알려 주세요.';
    case 'EXPIRED': return '주문 등록이 만료되었습니다. 매장 담당자에게 새 등록서를 받으세요.';
    default: return '서버에 연결하지 못했습니다. 인터넷 연결을 확인하고 다시 시도하세요.';
  }
}

function state(t: Say, token: FaucetToken, won?: true) {
  if (token.state === 'open') return t('보내는 중 · 확정되면 지갑에 보입니다');
  if (token.state === 'limit') return token.availableAt ? `${new Date(token.availableAt * 1000).toLocaleString('ko-KR')} ${t('이후 다시 받을 수 있습니다')}` : t('오늘 받을 수 있는 만큼 받았습니다');
  if (token.state === 'empty') return t('줄 수 있는 시험 토큰이 모자랍니다. 운영자에게 알려 주세요.');
  const last = token.claim?.state === 'failed' ? ` · ${t('지난번은 보내지 못했습니다')}` : token.claim?.state === 'unknown' ? ` · ${t('지난번 결과를 확인하지 못했습니다')}` : '';
  return `${t('한 번에')} ${tokenText(token.amount, {...token, won})} · ${t('남은 횟수')} ${token.remaining}${last}`;
}

/** 시험 토큰 받기: what the store's faucet gives, sent to the board account enrolled there. */
export function FaucetPage({t, account, sends, won = {}, gasCoin}: {
  t: Say; account: string;
  /** Token contracts the board can send. Others are not taken: they would be stuck on the board account. */
  sends: string[]; /** The store's won and its name in the wallet. */ won?: Record<string, string>; gasCoin?: string;
}) {
  const [status, setStatus] = useState<FaucetStatus>(), [problem, setProblem] = useState(''), [busy, setBusy] = useState(false), [said, setSaid] = useState('');
  // A request whose reply was lost is asked again under the same key, so it is never paid twice.
  const keys = useRef(new Map<string, string>());
  const read = useCallback(async () => {
    const {client} = await credential();
    const next = await client.faucetStatus();
    setStatus(next);
    setProblem('');
    for (const token of next.tokens) if (token.state === 'open' || token.claim?.state === 'final') keys.current.delete(token.token);
    return next;
  }, []);
  const refresh = useCallback(() => { read().catch(error => setProblem(faucetFailure(error))); }, [read]);
  useEffect(refresh, [refresh]);
  const open = !!status?.tokens.some(token => token.state === 'open');
  useEffect(() => {
    if (!open) return;
    const timer = setInterval(refresh, 10_000);
    return () => clearInterval(timer);
  }, [open, refresh]);
  const named = (token: FaucetToken) => won[token.token] ?? (token.token === stableNetCoin.asset ? stableNetCoin.label : token.symbol);
  const claim = async (token: FaucetToken) => {
    if (busy) return;
    setBusy(true);
    setSaid('');
    try {
      const {client} = await credential();
      const key = keys.current.get(token.token) ?? await walletId();
      keys.current.set(token.token, key);
      await client.claimFaucet(token.token, key);
      keys.current.delete(token.token);
      setSaid(`${named(token)} · ${t('보내 달라고 했습니다. 확정되면 지갑에 보입니다.')}`);
    } catch (error) {
      // Anything the server named is an answer; only silence leaves the request open.
      if (faucetFailure(error) !== faucetFailure(new Error())) keys.current.delete(token.token);
      setSaid(t(faucetFailure(error)));
    }
    await read().catch(() => {});
    setBusy(false);
  };
  const other = !!status && status.payer !== account;
  return <View style={styles.page} testID="faucet">
    <Text accessibilityRole="header" style={type.title}>{t('시험 토큰 받기')}</Text>
    <Text style={type.body}>{t('시험용 토큰입니다. 실제 돈이 아니고 바꿀 수 없습니다.')}</Text>
    {!!said && <Text accessibilityLiveRegion="polite" style={styles.said}>{said}</Text>}
    {!!problem && <><Text accessibilityLiveRegion="polite" style={styles.warn}>{t(problem)}</Text><KitButton label={t('다시 확인')} run={refresh} variant="secondary" /></>}
    {!status && !problem && <Text style={type.body}>{t('확인하는 중')}</Text>}
    {status && other && <Text style={styles.warn}>{t(routeWords(status.payer ? '다른 보드 계정이 이 매장에 등록되어 있습니다. 설정에서 매장 등록을 다시 하세요.' : '이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.'))}</Text>}
    {status && <>
      <View style={styles.list}>{status.tokens.map(token => {
        const taken = sends.includes(token.token);
        return <View key={token.token} style={styles.line}>
          <View style={styles.words}><Text style={type.label}>{named(token)}</Text>
            <Text style={type.small}>{taken ? state(t, token, won[token.token] ? true : undefined) : t('보드가 아직 이 토큰을 보낼 수 없어 받지 않습니다')}</Text></View>
          {taken && token.state === 'ready' && !other ? <View style={styles.claim}><KitButton label={`${named(token)} ${t('받기')}`} run={() => { claim(token).catch(() => {}); }} disabled={busy} /></View>
            : <Chip label={t(!taken ? '사용 안 함' : token.state === 'open' ? '보내는 중' : token.state === 'limit' ? '받음' : token.state === 'empty' ? '모자람' : '확인 필요')} tone={token.state === 'open' ? 'wait' : 'off'} />}
        </View>;
      })}</View>
      <Card><Row label={t('받는 계정')} value={`${account.slice(0, 6)}…${account.slice(-4)}`} /><Row label={t('수수료')} value={t('운영 서버가 냅니다')} /></Card>
    </>}
    <Text style={type.small}>{t('매장 결제와 반납에는 가스 코인이 필요하지 않습니다.')}</Text>
    {!!gasCoin && <KitButton label={t('가스 코인 받는 곳 열기')} variant="secondary" run={() => { Linking.openURL(gasCoin).catch(() => {}); }} />}
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14}, words: {flex: 1, gap: 2}, claim: {minWidth: 120},
  list: {borderRadius: radius.l, borderWidth: 1, borderColor: color.line, overflow: 'hidden', gap: 1, backgroundColor: color.line},
  line: {flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 72, paddingHorizontal: 14, paddingVertical: 10, backgroundColor: color.sheet},
  said: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.deep, backgroundColor: color.mintSoft, borderRadius: radius.m, padding: 12},
  warn: {fontFamily: font, fontSize: 14, lineHeight: 21, color: '#5a3b05', backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12},
});
