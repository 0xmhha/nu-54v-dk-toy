import React, {useEffect, useState} from 'react';
import {Pressable, ScrollView, StatusBar, StyleSheet, Text, View} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';
import NativeWallet from '../specs/NativeOwnerWallet.ts';
import {KitButton} from '../ui/kit.tsx';
import {useInnerBack} from '../ui/innerBack.ts';
import {color, font, radius, type} from '../ui/theme.ts';
import {decodeWalletStorage} from '../wallet/profile.ts';
import {WalletProfileStore} from '../wallet/walletStorage.ts';
import {missingConsents, noteConsents, requiredConsents, withConsent, type Consent} from './consent.ts';
import {NoticePage} from './LegalScreens.tsx';
import {notice, type NoticeId} from './notices.ts';

const words = {
  ko: {title: '시작하기 전에', lead: '시험용 앱입니다. 실제 돈으로 바꿀 수 있는 자산을 다루지 않습니다.', required: '필수', optional: '선택', view: '보기',
    alerts: '주문 준비 알림', alertsDetail: '동의하지 않아도 결제할 수 있습니다', start: '동의하고 시작', back: '돌아가기', language: 'English',
    permissions: '휴대전화 권한은 쓸 때 묻습니다', permissionsDetail: '블루투스는 보드와 근처 매장에 연결할 때, 카메라는 QR을 읽을 때, 알림은 주문 준비를 알릴 때 씁니다.',
    keep: '동의한 문서의 판과 시각은 이 휴대전화에 저장합니다. 문서가 바뀌면 다시 묻습니다.', failed: '동의를 저장하지 못했습니다. 다시 시도하세요.',
    responsibility: '보드를 초기화하거나 잃으면 열쇠와 남은 돈을 되찾을 수 없습니다'},
  en: {title: 'Before you start', lead: 'This is a test app. It handles no asset that can be exchanged for real money.', required: 'required', optional: 'optional', view: 'Read',
    alerts: 'Order-ready notices', alertsDetail: 'You can pay without agreeing to this', start: 'Agree and start', back: 'Back', language: '한국어',
    permissions: 'Phone permissions are asked when needed', permissionsDetail: 'Bluetooth for the board and nearby shops, camera for QR codes, notifications for ready orders.',
    keep: 'The edition and time you agreed to are kept on this phone. You are asked again when a document changes.', failed: 'Could not save your agreement. Try again.',
    responsibility: 'A reset or lost board loses its key and the money on it'},
};

function Check({label, detail, on, toggle, view}: {label: string; detail: string; on: boolean; toggle: () => void; view?: {label: string; run: () => void}}) {
  return <View style={styles.row}>
    <Pressable accessibilityRole="checkbox" accessibilityLabel={label} accessibilityState={{checked: on}} onPress={toggle} style={styles.check}>
      <View style={[styles.box, on && styles.boxOn]}>{on && <View style={styles.tick} />}</View>
      <View style={styles.words}><Text style={type.label}>{label}</Text><Text style={type.small}>{detail}</Text></View>
    </Pressable>
    {view && <Pressable accessibilityRole="button" accessibilityLabel={`${label} ${view.label}`} onPress={view.run} style={styles.view}><Text style={styles.viewText}>{view.label}</Text></Pressable>}
  </View>;
}

/** Asks for the required agreements once, and again when a document's edition changes. */
export function ConsentGate({children}: {children: React.ReactNode}) {
  const [asking, setAsking] = useState<boolean>();
  const [language, setLanguage] = useState<'ko' | 'en'>('ko');
  const [ticked, setTicked] = useState<string[]>([]);
  const [reading, setReading] = useState<NoticeId>();
  const [busy, setBusy] = useState(false), [failed, setFailed] = useState(false);
  useInnerBack(!!reading, () => setReading(undefined));
  useEffect(() => {
    let live = true;
    const open = (consents?: Consent[]) => { noteConsents(consents); if (live) setAsking(missingConsents(consents).length > 0); };
    if (!NativeWallet) { open(); setAsking(false); return; }
    NativeWallet.loadProfile().then(raw => {
      const record = decodeWalletStorage(raw);
      if (live && record.language) setLanguage(record.language);
      open(record.consents);
    // A record that cannot be read cannot keep an agreement either; the wallet says so itself.
    }).catch(() => { if (live) setAsking(false); });
    return () => { live = false; };
  }, []);
  if (asking === false) return <>{children}</>;
  const say = words[language];
  const toggle = (id: string) => setTicked(ticked.includes(id) ? ticked.filter(item => item !== id) : [...ticked, id]);
  const start = async () => {
    setBusy(true);
    setFailed(false);
    try {
      const store = new WalletProfileStore(NativeWallet!), head = await store.load(), at = Date.now();
      let consents = head.consents;
      for (const id of requiredConsents) consents = withConsent(consents, id, true, at);
      consents = withConsent(consents, 'alerts', ticked.includes('alerts'), at);
      await store.save(head, {...head, consents, language});
      noteConsents(consents);
      setAsking(false);
    } catch { setFailed(true); } finally { setBusy(false); }
  };
  const document = reading && notice(reading, language);
  return <SafeAreaProvider><StatusBar barStyle="dark-content" /><SafeAreaView style={styles.root}>
    {asking && <ScrollView contentContainerStyle={styles.content} key={reading ?? 'list'}>
      {document ? <><NoticePage document={document} language={language} /><KitButton label={say.back} run={() => setReading(undefined)} variant="secondary" /></> : <>
        <View style={styles.head}><Text accessibilityRole="header" style={[type.title, styles.words]}>{say.title}</Text>
          <Pressable accessibilityRole="button" accessibilityLabel={say.language} onPress={() => setLanguage(language === 'ko' ? 'en' : 'ko')} style={styles.view}>
            <Text style={styles.viewText}>{say.language}</Text></Pressable></View>
        <Text style={type.body}>{say.lead}</Text>
        <View style={styles.list}>
          {requiredConsents.map(id => {
            const item = notice(id, language);
            return item && <Check key={id} label={`${item.title} (${say.required})`} on={ticked.includes(id)} toggle={() => toggle(id)}
              detail={id === 'test-assets' ? say.responsibility : `${item.edition}${item.status === 'draft' ? (language === 'ko' ? ' · 초안' : ' · draft') : ''}`}
              view={{label: say.view, run: () => setReading(id)}} />;
          })}
          <Check label={`${say.alerts} (${say.optional})`} detail={say.alertsDetail} on={ticked.includes('alerts')} toggle={() => toggle('alerts')} />
        </View>
        <View style={styles.note}><Text style={type.label}>{say.permissions}</Text><Text style={type.small}>{say.permissionsDetail}</Text></View>
        {failed && <Text accessibilityLiveRegion="polite" style={styles.failed}>{say.failed}</Text>}
        <KitButton label={say.start} run={() => { start().catch(() => {}); }} disabled={busy || requiredConsents.some(id => notice(id, language) && !ticked.includes(id))} />
        <Text style={type.small}>{say.keep}</Text>
      </>}
    </ScrollView>}
  </SafeAreaView></SafeAreaProvider>;
}

const styles = StyleSheet.create({
  root: {flex: 1, backgroundColor: color.ground},
  content: {gap: 16, padding: 16, paddingBottom: 32},
  head: {flexDirection: 'row', alignItems: 'center', gap: 8, paddingTop: 12},
  list: {borderRadius: radius.l, borderWidth: 1, borderColor: color.line, overflow: 'hidden', gap: 1, backgroundColor: color.line},
  row: {flexDirection: 'row', alignItems: 'center', backgroundColor: color.sheet, paddingLeft: 14, paddingRight: 4},
  check: {flex: 1, flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 64, paddingVertical: 10},
  box: {width: 24, height: 24, borderRadius: 7, borderWidth: 2, borderColor: color.mute, alignItems: 'center', justifyContent: 'center'},
  boxOn: {backgroundColor: color.green, borderColor: color.green},
  tick: {width: 7, height: 12, borderRightWidth: 2.5, borderBottomWidth: 2.5, borderColor: '#ffffff', transform: [{rotate: '45deg'}], marginTop: -3},
  words: {flex: 1, gap: 2},
  view: {minWidth: 48, minHeight: 48, paddingHorizontal: 10, alignItems: 'center', justifyContent: 'center'},
  viewText: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.green},
  note: {gap: 4, padding: 14, borderRadius: radius.m, backgroundColor: color.mintSoft},
  failed: {fontFamily: font, fontSize: 15, color: color.danger},
});
