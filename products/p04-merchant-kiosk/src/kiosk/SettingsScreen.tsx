import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { BackHandler, FlatList, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import {
  paysThroughEscrow,
  terminalIdleSeconds,
  type CommerceClient,
  type CommercePaymentProfile,
  type TerminalDevice,
  type TerminalDeviceChange,
  type TerminalDeviceName,
} from '@nu54/protocol';
import { checkReadServer } from '../chain/readServer.ts';
import { appVersion, licenseText, notice, notices, type License, type NoticeId } from '../legal/notices.ts';

/** Without a touch the screen returns to the menu after this long. */
export const settingsIdleMs = 60_000;
const documents: NoticeId[] = ['terms', 'privacy', 'test-assets'];
const reference = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
type Section = 'shop' | 'device' | 'payment';
type Page = { kind: 'document'; id: NoticeId } | { kind: 'licenses' } | { kind: 'license'; license: License };
type Locale = 'ko' | 'en';

const settingNames: Record<TerminalDeviceName, string> = {
  'read-server': '읽기 서버', language: '첫 화면 언어', 'idle-seconds': '대기 시간',
};
function settingValue(name: TerminalDeviceName, value: string) {
  if (name === 'language') return value === 'en' ? 'English' : '한국어';
  if (name === 'idle-seconds') return `${value}초`;
  return value || '운영 서버 주소';
}
const when = (seconds: number) => new Date(seconds * 1000).toLocaleString('ko-KR');

function Action({ label, run, disabled = false, chosen = false }: { label: string; run: () => void; disabled?: boolean; chosen?: boolean }) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{ disabled, selected: chosen }}
      onPress={run} disabled={disabled}
      style={({ pressed }) => [s.button, chosen && s.chosen, pressed && s.pressed, disabled && !chosen && s.disabled]}>
      <Text style={[s.buttonText, chosen && s.chosenText]}>{label}</Text>
    </Pressable>
  );
}

function Row({ name, value }: { name: string; value: string }) {
  return (
    <View style={s.row}>
      <Text style={s.hint}>{name}</Text>
      <Text style={s.label} selectable>{value}</Text>
    </View>
  );
}

function Title({ children }: { children: ReactNode }) {
  return <Text accessibilityRole="header" style={s.title}>{children}</Text>;
}

const licenseHeight = 64;

export function SettingsScreen({ client, terminal, policy, shopLabel, device, staff, target = '', locale: first = 'ko', requestId, close }: {
  client?: CommerceClient;
  /** This device is the customer terminal of the store. */
  terminal: boolean;
  policy?: CommercePaymentProfile;
  shopLabel?: string;
  device?: TerminalDevice;
  /** The signed-in operator holds terminal_settings. */
  staff: boolean;
  target?: string;
  locale?: Locale;
  requestId: () => Promise<string>;
  close: () => void;
}) {
  const [section, setSection] = useState<Section>('shop');
  const [page, setPage] = useState<Page>();
  const [locale, setLocale] = useState<Locale>(first);
  const ko = locale === 'ko';
  const [shop, setShop] = useState(shopLabel);
  const [network, setNetwork] = useState<'checking' | 'ok' | 'failed'>();
  const [kiosk, setKiosk] = useState(target);
  const [found, setFound] = useState<{ terminal: string; device: TerminalDevice }>();
  const [address, setAddress] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const live = useRef(true);
  const working = useRef(false);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  const latestClose = useRef(close);
  latestClose.current = close;
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const touch = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(function idle() {
      // A change on its way to the server is not left without an answer.
      if (working.current) timer.current = setTimeout(idle, settingsIdleMs);
      else latestClose.current();
    }, settingsIdleMs);
  }, []);
  useEffect(() => {
    touch();
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [touch]);

  const back = useCallback(() => {
    if (page?.kind === 'license') setPage({ kind: 'licenses' });
    else if (page) setPage(undefined);
    else latestClose.current();
  }, [page]);
  useEffect(() => {
    const listener = BackHandler.addEventListener('hardwareBackPress', () => {
      touch();
      back();
      return true;
    });
    return () => listener.remove();
  }, [back, touch]);

  useEffect(() => {
    if (shopLabel || !terminal || !client) return;
    client.publishedMenu().then(menu => live.current && setShop(menu.shop?.branchLabel), () => undefined);
  }, [client, terminal, shopLabel]);

  const server = device?.readServer ?? policy?.rpc;
  useEffect(() => {
    if (!terminal || !server) return;
    let open = true;
    setNetwork('checking');
    checkReadServer(server).then(() => open && setNetwork('ok'), () => open && setNetwork('failed'));
    return () => {
      open = false;
    };
  }, [terminal, server]);

  async function work(task: () => Promise<void>) {
    if (working.current) return;
    working.current = true;
    setBusy(true);
    setMessage('');
    try {
      await task();
    } finally {
      working.current = false;
      if (live.current) setBusy(false);
    }
  }

  async function find() {
    if (!client) return;
    const wanted = kiosk.trim();
    setFound(undefined);
    try {
      const answer = await client.terminalDevice(wanted);
      if (!live.current) return;
      setFound({ terminal: wanted, device: answer });
      setAddress(answer.readServer ?? '');
    } catch {
      if (live.current) setMessage('이 단말의 설정을 확인할 수 없습니다. 단말 식별값과 직원 권한, 서버 연결을 확인하세요.');
    }
  }

  async function change(name: TerminalDeviceName, value: string) {
    if (!client || !found) return;
    try {
      const next = await client.changeTerminalDevice(found.terminal, name, value, found.device.revision, await requestId());
      if (!live.current) return;
      setFound({ terminal: found.terminal, device: next });
      setAddress(next.readServer ?? '');
      setMessage('바꾼 내용을 운영 서버에 기록했습니다. 단말은 다음 주문부터 적용합니다.');
    } catch {
      if (!live.current) return;
      // The reply may be lost after the server recorded the change; only the server knows.
      await find();
      if (live.current) setMessage('운영 서버에서 변경을 확인하지 못했습니다. 화면의 값은 서버에 기록된 값입니다.');
    }
  }

  async function changeServer() {
    const value = address.trim();
    try {
      await checkReadServer(value);
    } catch (error) {
      const reason = error instanceof Error ? error.message : '';
      if (live.current) setMessage(reason === 'READ_SERVER_ADDRESS' ? 'https로 시작하고 계정 정보나 조회 문자열이 없는 주소를 입력하세요.'
        : reason === 'READ_SERVER_CHAIN' ? '이 서버는 매장의 결제 네트워크가 아닙니다. 저장하지 않았습니다.'
          : '이 서버가 응답하지 않습니다. 저장하지 않았습니다.');
      return;
    }
    if (live.current) await change('read-server', value);
  }

  const locked = (
    <Text style={s.label}>
      {terminal
        ? '매장 설정 권한이 있는 직원이 직원 등록 기기에서 로그인한 뒤 이 단말을 조회해 바꿉니다.'
        : '매장 설정 권한이 있는 직원만 열 수 있습니다. 직원 인증을 먼저 하세요.'}
    </Text>
  );
  const picker = (
    <>
      <Text style={s.hint}>단말 식별값</Text>
      <TextInput accessibilityLabel="설정할 단말 식별값" autoCapitalize="none" autoCorrect={false} maxLength={36} style={s.input}
        editable={!busy} value={kiosk} onChangeText={value => {
          touch();
          setKiosk(value);
          setFound(undefined);
        }} />
      <Action label="단말 설정 조회" disabled={busy || !reference.test(kiosk.trim())} run={() => void work(find)} />
    </>
  );

  function shopSection() {
    const name = shop;
    const escrow = policy && paysThroughEscrow(policy);
    return (
      <ScrollView key="shop" contentContainerStyle={s.content}>
        <Title>{ko ? '매장과 법적 고지' : 'Store and legal notices'}</Title>
        {name && <Row name={ko ? '매장' : 'Store'} value={name} />}
        {notices.publisher.name && <Row name={ko ? '앱 제공자' : 'App provider'} value={notices.publisher.name} />}
        {notices.publisher.contact && <Row name={ko ? '문의' : 'Contact'} value={notices.publisher.contact} />}
        {terminal && network && (
          <Row name={ko ? '결제 네트워크' : 'Payment network'} value={`StableNet ${ko ? '테스트넷' : 'Testnet'} · ${
            network === 'ok' ? (ko ? '연결됨' : 'connected') : network === 'checking' ? (ko ? '확인 중' : 'checking') :
              (ko ? '연결을 확인하지 못함' : 'connection not confirmed')}`} />
        )}
        {policy && (
          <>
            <Text accessibilityRole="header" style={s.heading}>{ko ? '결제와 환불' : 'Payment and refunds'}</Text>
            <Text style={s.label}>
              {ko
                ? '주문 금액은 휴대전화 지갑과 하드월렛에서 확인하고 승인합니다. 네트워크 수수료는 매장이 냅니다.'
                : 'You check and approve the amount on your phone wallet and hardware wallet. The store pays the network fee.'}
            </Text>
            <Text style={s.label}>
              {escrow
                ? ko
                  ? '결제한 금액은 주문 보호 계약이 보관합니다. 매장이 전달한 뒤 고객이 수령을 확인하거나 확인 기간이 지나면 매장에 지급됩니다. 매장이 이행하지 못하거나 전달 기한이 지나면 결제한 지갑으로 돌아갑니다. 확인 기간 안에는 휴대전화에서 이의를 낼 수 있습니다.'
                  : 'The order protection contract holds your payment. The store is paid after you confirm receipt or the confirmation period ends. If the store cannot deliver, or the delivery deadline passes, the money returns to the wallet that paid. You can dispute the order on your phone during the confirmation period.'
                : ko
                  ? '환불은 매장이 확인하고 승인한 뒤 결제한 지갑으로 보냅니다. 주문번호를 매장 직원에게 알려 주세요.'
                  : 'The store reviews refunds and returns the money to the wallet that paid. Show your order number to the staff.'}
            </Text>
          </>
        )}
        <Text accessibilityRole="header" style={s.heading}>{ko ? '고지' : 'Notices'}</Text>
        {documents.map(id => {
          const item = notice(id, locale);
          return item && <Action key={id} label={item.title} run={() => setPage({ kind: 'document', id })} />;
        })}
        {notices.licenses.length > 0 &&
          <Action label={ko ? '오픈소스 라이선스' : 'Open source licenses'} run={() => setPage({ kind: 'licenses' })} />}
        <Row name={ko ? '앱 버전' : 'App version'} value={appVersion} />
        <Action label={ko ? 'English' : '한국어'} run={() => setLocale(ko ? 'en' : 'ko')} />
      </ScrollView>
    );
  }

  function deviceSection() {
    const current = found?.device;
    const reset = current?.readServer !== undefined;
    return (
      <ScrollView key="device" contentContainerStyle={s.content} keyboardShouldPersistTaps="handled">
        <Title>기기</Title>
        {!staff ? locked : (
          <>
            {picker}
            {current && (
              <>
                <Text accessibilityRole="header" style={s.heading}>첫 화면 언어</Text>
                <View style={s.choices}>
                  {(['ko', 'en'] as const).map(value => <Action key={value} label={value === 'ko' ? '한국어' : 'English'}
                    chosen={current.language === value} disabled={busy || current.language === value}
                    run={() => void work(() => change('language', value))} />)}
                </View>
                <Text accessibilityRole="header" style={s.heading}>주문 화면 대기 시간</Text>
                <View style={s.choices}>
                  {terminalIdleSeconds.map(value => <Action key={value} label={`${value}초`} chosen={current.idleSeconds === value}
                    disabled={busy || current.idleSeconds === value} run={() => void work(() => change('idle-seconds', String(value)))} />)}
                </View>
                <Text accessibilityRole="header" style={s.heading}>읽기 서버</Text>
                <Text style={s.hint}>체인 상태를 조회하는 데만 쓰며 결제 제출에는 쓰지 않습니다. 저장하기 전에 결제 네트워크가 맞는지 확인합니다.</Text>
                <Row name="현재" value={current.readServer ?? '운영 서버 주소'} />
                <TextInput accessibilityLabel="읽기 서버 주소" placeholder="https://" autoCapitalize="none" autoCorrect={false} keyboardType="url"
                  maxLength={512} style={s.input} editable={!busy} value={address} onChangeText={value => {
                    touch();
                    setAddress(value);
                  }} />
                <View style={s.choices}>
                  <Action label="확인하고 저장" disabled={busy || !address.trim() || address.trim() === current.readServer} run={() => void work(changeServer)} />
                  <Action label="운영 서버 주소로 되돌리기" disabled={busy || !reset} run={() => void work(() => change('read-server', ''))} />
                </View>
                <Text accessibilityRole="header" style={s.heading}>변경 기록</Text>
                {!current.changes?.length && <Text style={s.label}>기록된 변경이 없습니다.</Text>}
                {current.changes?.map((item: TerminalDeviceChange) => (
                  <Text key={`${item.at}/${item.name}/${item.to}`} style={s.label}>
                    {`${when(item.at)} · 직원 ${item.actor.slice(-8)} · ${settingNames[item.name]} · ${
                      settingValue(item.name, item.from)} → ${settingValue(item.name, item.to)}`}
                  </Text>
                ))}
              </>
            )}
          </>
        )}
      </ScrollView>
    );
  }

  function paymentSection() {
    const payment = found?.device.payment;
    return (
      <ScrollView key="payment" contentContainerStyle={s.content} keyboardShouldPersistTaps="handled">
        <Title>결제</Title>
        {!staff ? locked : (
          <>
            <Text style={s.hint}>운영 서버에 등록된 값입니다. 키오스크에서는 바꿀 수 없습니다.</Text>
            {picker}
            {found && !payment && <Text style={s.label}>운영 서버에 이 매장의 결제 값이 없습니다.</Text>}
            {payment && (
              <>
                <Row name="네트워크" value={`StableNet 테스트넷 (${payment.chainId})`} />
                <Row name="토큰" value={payment.asset} />
                <Row name="매장 수령 주소" value={payment.beneficiary} />
                <Row name={payment.escrow ? '주문 보호 계약' : '결제 계약'} value={payment.contract} />
                <Row name="가스 대납 계약" value={payment.gasSponsor ?? '사용하지 않음'} />
                {payment.gasAccount && <Row name="가스 계정" value={payment.gasAccount} />}
                <Row name="단말 등록 만료" value={when(payment.registrationExpiresAt)} />
              </>
            )}
          </>
        )}
      </ScrollView>
    );
  }

  function openPage(current: Page) {
    if (current.kind === 'licenses') {
      return (
        <FlatList testID="license-list" style={s.fill} contentContainerStyle={s.list} data={notices.licenses}
          keyExtractor={(item, index) => `${item.name}@${item.version}/${index}`}
          initialNumToRender={12} maxToRenderPerBatch={20} windowSize={5} removeClippedSubviews
          getItemLayout={(_, index) => ({ length: licenseHeight, offset: licenseHeight * index, index })}
          onScrollBeginDrag={touch}
          renderItem={({ item }) => (
            <Pressable accessibilityRole="button" accessibilityLabel={`${item.name} ${item.version} ${item.license}`} style={s.license}
              onPress={() => setPage({ kind: 'license', license: item })}>
              <Text numberOfLines={1} style={s.label}>{`${item.name} ${item.version}`}</Text>
              <Text numberOfLines={1} style={s.hint}>{item.license}</Text>
            </Pressable>
          )} />
      );
    }
    if (current.kind === 'license') {
      return (
        <ScrollView key={current.license.name} style={s.fill} contentContainerStyle={s.content} onScrollBeginDrag={touch}>
          <Title>{`${current.license.name} ${current.license.version}`}</Title>
          <Text style={s.hint}>{current.license.license}</Text>
          <Text style={s.legal}>{licenseText(current.license)}</Text>
        </ScrollView>
      );
    }
    const item = notice(current.id, locale);
    return (
      <ScrollView key={current.id} style={s.fill} contentContainerStyle={s.content} onScrollBeginDrag={touch}>
        {item && (
          <>
            <Title>{item.title}</Title>
            {item.status === 'draft' && <Text style={s.notice}>{ko ? '검토 중인 초안입니다.' : 'This is a draft under review.'}</Text>}
            {item.effective && <Text style={s.hint}>{`${ko ? '시행일' : 'Effective'} ${item.effective}`}</Text>}
            {item.sections.map((part, index) => (
              <View key={index} style={s.part}>
                <Text accessibilityRole="header" style={s.heading}>{part.heading}</Text>
                <Text style={s.legal}>{part.body}</Text>
              </View>
            ))}
          </>
        )}
      </ScrollView>
    );
  }

  const tabs: [Section, string][] = [['shop', ko ? '매장과 법적 고지' : 'Store and legal notices'], ['device', '기기'], ['payment', '결제']];
  return (
    <SafeAreaView style={s.root}>
      <View style={s.body} testID="kiosk-settings" onStartShouldSetResponderCapture={() => {
        touch();
        return false;
      }}>
        <View style={s.side}>
          <Text accessibilityRole="header" style={s.title}>{ko ? '설정' : 'Settings'}</Text>
          {tabs.map(([id, label]) => <Action key={id} label={label} chosen={section === id && !page} run={() => {
            setPage(undefined);
            setMessage('');
            setSection(id);
          }} />)}
          <View style={s.fill} />
          {page && <Action label={ko ? '뒤로' : 'Back'} run={back} />}
          <Action label={ko ? '설정 닫기' : 'Close settings'} run={close} />
          <Text style={s.hint}>{ko ? '1분 동안 사용하지 않으면 자동으로 닫힙니다.' : 'Closes after one minute without use.'}</Text>
        </View>
        <View style={s.fill}>
          {!!message && !page && section !== 'shop' && <Text accessibilityLiveRegion="polite" style={s.notice}>{message}</Text>}
          {page ? openPage(page) : section === 'shop' ? shopSection() : section === 'device' ? deviceSection() : paymentSection()}
        </View>
      </View>
    </SafeAreaView>
  );
}

const ink = '#182d25';
const line = '#d5dfd3';
const paper = '#fffefa';
const deep = '#174b38';

const s = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#f1f4ec' },
  body: { flex: 1, flexDirection: 'row' },
  side: { width: 260, padding: 24, gap: 12, backgroundColor: paper, borderRightWidth: 1, borderRightColor: line },
  fill: { flex: 1 },
  content: { padding: 28, gap: 14 },
  list: { paddingHorizontal: 28 },
  title: { fontSize: 28, color: ink, fontWeight: '800', letterSpacing: -0.6 },
  heading: { fontSize: 20, color: ink, fontWeight: '800' },
  label: { fontSize: 17, color: ink, lineHeight: 25 },
  hint: { fontSize: 15, color: '#4f655a', lineHeight: 22 },
  legal: { fontSize: 15, color: ink, lineHeight: 23 },
  notice: { fontSize: 16, color: '#5a3b05', lineHeight: 23, backgroundColor: '#f6e3bf', borderRadius: 12, padding: 14, margin: 12 },
  row: { borderBottomWidth: 1, borderBottomColor: line, paddingBottom: 10, gap: 2 },
  part: { gap: 6 },
  choices: { flexDirection: 'row', flexWrap: 'wrap', gap: 12 },
  license: { height: licenseHeight, justifyContent: 'center', borderBottomWidth: 1, borderBottomColor: line },
  input: { borderWidth: 1, borderColor: '#71897d', borderRadius: 12, minHeight: 52, padding: 12, fontSize: 16, color: ink, backgroundColor: paper },
  button: {
    minHeight: 52, borderRadius: 14, borderWidth: 1, borderColor: line, backgroundColor: paper,
    paddingHorizontal: 18, paddingVertical: 12, justifyContent: 'center', alignItems: 'center',
  },
  chosen: { backgroundColor: deep, borderColor: deep },
  pressed: { opacity: 0.75 },
  disabled: { opacity: 0.5 },
  buttonText: { fontSize: 16, color: ink, fontWeight: '700', textAlign: 'center' },
  chosenText: { color: '#f4f8ed' },
});
