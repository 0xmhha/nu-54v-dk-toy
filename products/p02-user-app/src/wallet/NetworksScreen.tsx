import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import {Card, Chip, KitButton, Row, Section} from '../ui/kit.tsx';
import {useInnerBack, usePageTop} from '../ui/innerBack.ts';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import {defaultReadState, type ReadProfileState} from './readProfiles.ts';
import {
  addCustom, boardNetwork, changeBuiltIn, checkServer, describeCustom, isReadProfile, networkRows, removeCustom, serverFailure,
  type CustomDraft, type Exchange, type NetworkAdvice, type NetworkRow, type ServerSource,
} from './networks.ts';

type Say = (ko: string) => string;
type Health = 'checking' | 'ok' | 'failed';
type Props = {
  t: Say; state?: ReadProfileState; advice?: NetworkAdvice; exchange?: Exchange; offline?: boolean; back: () => void;
  /** Applies a change to the newest stored networks. */
  save: (change: (state: ReadProfileState) => ReadProfileState) => Promise<unknown>;
  /** Scrolls the screen to its top. */
  top?: () => void;
};

const sources: Record<ServerSource, string> = {own: '직접 입력한 서버', store: '매장 등록서의 서버', operator: '운영자 목록의 서버', 'built-in': '앱의 기본 서버'};
const emptyDraft: CustomDraft = {name: '', server: '', chainId: '', currency: '', explorer: ''};

function Field({label, value, change, placeholder, numeric, editable}: {
  label: string; value: string; change: (text: string) => void; placeholder?: string; numeric?: boolean; editable: boolean;
}) {
  return <View style={styles.field}><Text style={styles.fieldLabel}>{label}</Text>
    <TextInput accessibilityLabel={label} value={value} onChangeText={change} placeholder={placeholder} placeholderTextColor={inputPlaceholder} style={styles.input}
      autoCapitalize="none" autoCorrect={false} maxLength={320} keyboardType={numeric ? 'number-pad' : 'default'} editable={editable} /></View>;
}

/** One server's answer, asked again when the page opens or the address changes. No account is sent. */
function useHealth(rows: NetworkRow[], exchange: Exchange | undefined, offline: boolean) {
  const [health, setHealth] = useState<Record<string, Health>>({});
  const latest = useRef(rows);
  latest.current = rows;
  const [round, setRound] = useState(0);
  const key = rows.map(row => `${row.key} ${row.server}`).join('\n');
  useEffect(() => {
    if (!exchange || offline) { setHealth({}); return; }
    let live = true;
    setHealth(Object.fromEntries(latest.current.map(row => [row.key, 'checking'])));
    for (const row of latest.current) {
      checkServer(row, row.server, exchange).then(() => 'ok' as const, () => 'failed' as const)
        .then(result => { if (live) setHealth(known => ({...known, [row.key]: result})); });
    }
    return () => { live = false; };
  }, [key, exchange, offline, round]);
  return {health, again: useCallback(() => setRound(value => value + 1), [])};
}

function Status({t, health}: {t: Say; health?: Health}) {
  return health === 'ok' ? <Chip label={t('정상')} tone="ok" /> : health === 'failed' ? <Chip label={t('연결 안 됨')} tone="wait" />
    : <Chip label={t(health === 'checking' ? '확인하는 중' : '확인 안 됨')} />;
}

function NetworkLine({t, row, health, open}: {t: Say; row: NetworkRow; health?: Health; open: () => void}) {
  const detail = row.hidden ? t('숨김') : row.custom ? `${t('직접 추가함')} · ${t('체인')} ${row.chainId}` : t(sources[row.source]);
  return <Pressable accessibilityRole="button" accessibilityLabel={`${row.label}, ${detail}`} onPress={open} style={({pressed}) => [styles.line, pressed && styles.pressed]}>
    <View style={styles.badge}><Text style={styles.badgeText}>{row.short}</Text></View>
    <View style={styles.words}><Text style={type.label}>{row.label}</Text><Text style={type.small}>{detail}</Text></View>
    <Status t={t} health={health} />
  </Pressable>;
}

function EditPage({t, row, exchange, save, done}: {t: Say; row: NetworkRow; exchange?: Exchange; save: Props['save']; done: (message: string) => void}) {
  const id = isReadProfile(row.key) ? row.key : undefined;
  const [own, setOwn] = useState(row.source === 'own' ? row.server : '');
  const [explorer, setExplorer] = useState(row.explorer ?? ''), [gasCoin, setGasCoin] = useState(row.gasCoin ?? '');
  const [busy, setBusy] = useState(false), [problem, setProblem] = useState(''), [removing, setRemoving] = useState(false);
  useInnerBack(removing, () => setRemoving(false));
  const apply = async (run: () => Promise<string>) => {
    if (busy) return;
    setBusy(true);
    setProblem('');
    try { done(await run()); } catch (error) { setProblem(t(serverFailure(error))); setBusy(false); }
  };
  const store = () => apply(async () => {
    const server = own.trim();
    const checked = server ? await checkServer(row, server, exchange!) : '';
    if (id) await save(state => changeBuiltIn(state, id, {server: checked, explorer, gasCoin}));
    else {
      if (!checked) throw new Error('WALLET_READ_HTTPS');
      await save(state => ({...state, custom: (state.custom ?? []).map(item => item.chainId === row.chainId ? {...item, rpc: checked} : item)}));
    }
    return '네트워크를 저장했습니다.';
  });
  if (removing) return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{t('네트워크 삭제')}</Text>
    <Card><Row label={t('이름')} value={row.label} /><Row label={t('체인 ID')} value={row.chainId ?? ''} /></Card>
    <Text style={type.section}>{t('함께 지워지는 것')}</Text>
    <Text style={type.body}>{t('이 휴대전화에 저장한 서버 주소와 네트워크 이름입니다.')}</Text>
    <Text style={type.section}>{t('지워지지 않는 것')}</Text>
    <Text style={type.body}>{t('보드의 열쇠와 체인의 잔액은 그대로입니다. 네트워크를 다시 추가하면 다시 보입니다.')}</Text>
    {!!problem && <Text accessibilityLiveRegion="polite" style={styles.problem}>{problem}</Text>}
    <Pressable accessibilityRole="button" accessibilityLabel={t('네트워크 삭제')} disabled={busy} style={({pressed}) => [styles.remove, (busy || pressed) && styles.pressed]}
      onPress={() => { apply(async () => { await save(state => removeCustom(state, row.chainId!)); return '네트워크를 삭제했습니다.'; }); }}>
      <Text style={styles.removeText}>{t('네트워크 삭제')}</Text></Pressable>
    <KitButton label={t('취소')} run={() => setRemoving(false)} variant="secondary" disabled={busy} />
  </View>;
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{row.label}</Text>
    <Card>
      {!!row.chainId && <Row label={t('체인 ID')} value={row.chainId} />}
      <Row label={t('보드')} value={t(row.sends ? '받기·보내기' : '보기만')} />
      <Row label={t('지금 쓰는 서버')} value={t(sources[row.source])} />
      <Text selectable style={type.small}>{row.server}</Text>
    </Card>
    <Field label={t('서버 주소 (https)')} value={own} change={text => { setOwn(text); setProblem(''); }} placeholder={id ? t('비우면 기본 서버를 씁니다') : 'https://'} editable={!busy} />
    {id && <Field label={t('탐색기 주소 (선택)')} value={explorer} change={setExplorer} placeholder="https://" editable={!busy} />}
    {id && <Field label={t('가스 코인 받는 곳 (선택)')} value={gasCoin} change={setGasCoin} placeholder="https://" editable={!busy} />}
    {!!problem && <Text accessibilityLiveRegion="polite" style={styles.problem}>{problem}</Text>}
    <KitButton label={t(busy ? '서버를 확인하는 중' : '저장')} run={store} disabled={busy || (!exchange && !!own.trim())} />
    {id && row.source === 'own' && <KitButton label={t('기본 서버로 되돌리기')} variant="secondary" disabled={busy}
      run={() => { apply(async () => { await save(state => changeBuiltIn(state, id, {server: ''})); return '기본 서버로 되돌렸습니다.'; }); }} />}
    {id && id !== boardNetwork && <KitButton label={t(row.hidden ? '목록에 다시 보이기' : '목록에서 숨기기')} variant="secondary" disabled={busy}
      run={() => { apply(async () => { await save(state => changeBuiltIn(state, id, {hidden: !row.hidden})); return row.hidden ? '네트워크를 다시 보이게 했습니다.' : '네트워크를 숨겼습니다.'; }); }} />}
    {row.custom && <KitButton label={t('네트워크 삭제')} run={() => setRemoving(true)} variant="secondary" disabled={busy} />}
    <Text style={type.small}>{t(id ? '앱에 있는 네트워크는 지울 수 없고 숨길 수만 있습니다. 서버를 바꿔도 보드의 열쇠와 결제받는 주소는 바뀌지 않습니다.'
      : '직접 추가한 네트워크에서는 잔액만 볼 수 있습니다. 보드가 보낼 수 없어 받기 주소를 보여 주지 않습니다.')}</Text>
  </View>;
}

function AddPage({t, exchange, save, done}: {t: Say; exchange?: Exchange; save: Props['save']; done: (message: string) => void}) {
  const [draft, setDraft] = useState(emptyDraft);
  const [busy, setBusy] = useState(false), [problem, setProblem] = useState('');
  const set = (key: keyof CustomDraft) => (text: string) => { setDraft({...draft, [key]: text}); setProblem(''); };
  const ready = !!draft.name.trim() && !!draft.server.trim() && !!draft.chainId.trim() && !!draft.currency.trim();
  const add = async () => {
    if (busy || !exchange) return;
    setBusy(true);
    setProblem('');
    try {
      const item = await describeCustom(draft, exchange);
      await save(state => addCustom(state, item));
      done('네트워크를 추가했습니다.');
    } catch (error) { setProblem(t(serverFailure(error))); setBusy(false); }
  };
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{t('네트워크 추가')}</Text>
    <Text style={type.small}>{t('이더리움 방식의 테스트넷을 추가합니다. 서버가 알려 주는 체인 ID와 첫 블록을 확인한 뒤 저장합니다.')}</Text>
    <Field label={t('네트워크 이름')} value={draft.name} change={set('name')} editable={!busy} />
    <Field label={t('서버 주소 (https)')} value={draft.server} change={set('server')} placeholder="https://" editable={!busy} />
    <Field label={t('체인 ID')} value={draft.chainId} change={set('chainId')} numeric editable={!busy} />
    <Field label={t('코인 기호')} value={draft.currency} change={set('currency')} placeholder="ETH" editable={!busy} />
    <Field label={t('탐색기 주소 (선택)')} value={draft.explorer} change={set('explorer')} placeholder="https://" editable={!busy} />
    <Text style={styles.warn}>{t('직접 추가한 네트워크에서는 잔액만 볼 수 있습니다. 보드가 보낼 수 없어 받기 주소를 보여 주지 않습니다.')}</Text>
    {!!problem && <Text accessibilityLiveRegion="polite" style={styles.problem}>{problem}</Text>}
    <KitButton label={t(busy ? '서버를 확인하는 중' : '확인하고 저장')} run={add} disabled={busy || !ready || !exchange} />
  </View>;
}

/** 설정 · 네트워크: every network of the wallet, its server, and the ones the owner added. */
export function NetworksScreen({t, state = defaultReadState, advice, exchange, offline = false, save, back, top}: Props) {
  const rows = networkRows(state, advice);
  const [open, setOpen] = useState<string>(), [message, setMessage] = useState('');
  const {health, again} = useHealth(open ? [] : rows.filter(row => !row.hidden), exchange, offline);
  const close = () => setOpen(undefined);
  useInnerBack(!!open, close);
  usePageTop(open, top);
  const done = (text: string) => { setMessage(t(text)); close(); };
  const row = rows.find(item => item.key === open);
  if (open) return <>
    {row ? <EditPage key={row.key} t={t} row={row} exchange={exchange} save={save} done={done} /> : <AddPage t={t} exchange={exchange} save={save} done={done} />}
    <KitButton label={t('네트워크 목록으로')} run={close} variant="secondary" />
  </>;
  const lines = (list: NetworkRow[]) => list.map(item => <NetworkLine key={item.key} t={t} row={item} health={item.hidden ? undefined : health[item.key]}
    open={() => { setMessage(''); setOpen(item.key); }} />);
  return <View style={styles.page} testID="networks">
    {!!message && <Text accessibilityLiveRegion="polite" style={type.body}>{message}</Text>}
    {offline && <Text style={styles.warn}>{t('인터넷에 연결되어 있지 않아 서버를 확인하지 못했습니다.')}</Text>}
    <Section title={t('보드로 받고 보낼 수 있음')}>{lines(rows.filter(item => item.sends))}</Section>
    <Section title={t('잔액 보기만')}>{lines(rows.filter(item => !item.sends))}</Section>
    <Text style={type.small}>{t('서버는 잔액과 거래를 읽는 곳입니다. 서버는 휴대전화의 접속 주소와 조회한 지갑 주소를 알 수 있습니다.')}</Text>
    <KitButton label={t('네트워크 추가')} run={() => { setMessage(''); setOpen('new'); }} />
    <KitButton label={t('다시 확인')} run={again} variant="secondary" disabled={offline || !exchange} />
    <KitButton label={t('돌아가기')} run={back} variant="secondary" />
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14}, words: {flex: 1, gap: 2},
  line: {flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 64, paddingHorizontal: 14, paddingVertical: 10, backgroundColor: color.sheet},
  pressed: {opacity: 0.6},
  badge: {minWidth: 40, height: 36, borderRadius: 18, paddingHorizontal: 6, backgroundColor: color.mintSoft, alignItems: 'center', justifyContent: 'center'},
  badgeText: {fontFamily: font, fontSize: 11, fontWeight: '800', color: color.deep},
  field: {gap: 6}, fieldLabel: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.mute},
  input: {fontFamily: font, fontSize: 15, borderWidth: 1, borderColor: '#8fa497', borderRadius: radius.m, padding: 12, minHeight: 50, color: color.ink, backgroundColor: color.sheet},
  problem: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.danger, backgroundColor: color.dangerBg, borderRadius: radius.m, padding: 12},
  warn: {fontFamily: font, fontSize: 14, lineHeight: 21, color: '#5a3b05', backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12},
  remove: {minHeight: 50, borderRadius: radius.m, backgroundColor: color.danger, alignItems: 'center', justifyContent: 'center'},
  removeText: {fontFamily: font, fontSize: 16, fontWeight: '700', color: '#ffffff'},
});
