/**
 * Renter phone app (P02 design 3): bond with the device from its label, then show what the
 * device is about to sign. There is no approve button: the renter approves on the device
 * (P02-FR-05), and enters the PIN on the device buttons (P02-FR-06).
 */

import { useEffect, useRef, useState } from 'react';
import { PermissionsAndroid, Platform, Pressable, StatusBar, StyleSheet, Text, TextInput, View } from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { fromBase64, GATT } from '@nu54/protocol';
import { ConfirmLink, type Screen } from './link/confirmLink.ts';
import { parseLabel, type BondTarget } from './qr.ts';
import RenterBle from './specs/NativeRenterBle.ts';

const OUTCOME_TEXT: Record<string, string> = {
  approved: '결제 완료',
  refused: '결제 거절',
  failed: '결제 실패',
  Checking: '확인 중',
};

async function connectPermission(): Promise<boolean> {
  if (Platform.OS !== 'android' || Number(Platform.Version) < 31) return true;
  const wanted = [PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT, PermissionsAndroid.PERMISSIONS.BLUETOOTH_SCAN];
  const got = await PermissionsAndroid.requestMultiple(wanted);
  return wanted.every(p => got[p] === PermissionsAndroid.RESULTS.GRANTED);
}

type Link = { state: 'none' } | { state: 'bonding' | 'connecting' | 'connected'; target: BondTarget } | { state: 'error'; message: string };

function Renter() {
  const [label, setLabel] = useState('');
  const [link, setLink] = useState<Link>({ state: 'none' });
  const [screen, setScreen] = useState<Screen>({ kind: 'waiting' });
  const confirm = useRef<ConfirmLink | null>(null);

  useEffect(() => () => confirm.current?.close(), []);

  const start = async () => {
    let target: BondTarget;
    try {
      target = parseLabel(label);
    } catch (e) {
      setLink({ state: 'error', message: String(e instanceof Error ? e.message : e) });
      return;
    }
    if (!(await connectPermission())) {
      setLink({ state: 'error', message: '블루투스 권한이 필요합니다' });
      return;
    }
    try {
      setLink({ state: 'bonding', target });
      if (!(await RenterBle.bond(target.address, target.passkey))) throw new Error('본딩하지 못했습니다. 기기를 페어링 모드로 두고 다시 시도하세요.');
      setLink({ state: 'connecting', target });
      await RenterBle.connect(target.address, GATT.service, GATT.rx, GATT.tx);
      let bonded = true;
      RenterBle.onBondLost(() => {
        bonded = false;
      });
      confirm.current?.close();
      confirm.current = new ConfirmLink(
        { bonded: () => bonded, onFragment: h => { const s = RenterBle.onFragment(b64 => h(fromBase64(b64))); return () => s.remove(); } },
        setScreen,
      );
      setLink({ state: 'connected', target });
    } catch (e) {
      setLink({ state: 'error', message: String(e instanceof Error ? e.message : e) });
    }
  };

  if (link.state !== 'connected') {
    return (
      <View>
        <Text style={styles.title}>기기 연결</Text>
        <Text style={styles.body}>기기 라벨의 QR 내용(nu54://bond?...)을 넣고, 기기를 길게 눌러 페어링 모드로 둔 뒤 연결하세요.</Text>
        <TextInput style={styles.input} value={label} onChangeText={setLabel} autoCapitalize="none" placeholder="nu54://bond?addr=...&passkey=..." />
        {link.state === 'bonding' && <Text style={styles.body}>본딩 중. 시스템 창이 뜨면 passkey {link.target.passkey}를 입력하세요.</Text>}
        {link.state === 'connecting' && <Text style={styles.body}>연결 중</Text>}
        {link.state === 'error' && <Text style={styles.error}>{link.message}</Text>}
        <Button label="연결" onPress={start} disabled={link.state === 'bonding' || link.state === 'connecting'} />
      </View>
    );
  }
  return (
    <View>
      {screen.kind === 'waiting' && (
        <>
          <Text style={styles.title}>결제 대기</Text>
          <Text style={styles.body}>키오스크에서 결제를 시작하면 기기가 보낸 결제 내용이 여기에 나옵니다.</Text>
          <Text style={styles.hint}>PIN이 필요할 때는 기기 버튼으로 입력하세요. LED가 자릿수와 누를 버튼을 안내합니다.</Text>
        </>
      )}
      {screen.kind === 'confirming' && (
        <>
          <Text style={styles.label}>가맹점</Text>
          <Text style={styles.value}>{screen.view.merchantName}</Text>
          <Text style={styles.label}>금액</Text>
          <Text style={styles.amount}>{screen.view.amount}</Text>
          {!screen.view.known && <Text style={styles.error}>알 수 없는 토큰 {screen.view.token}: 금액은 base unit입니다</Text>}
          <Text style={styles.label}>받는 주소</Text>
          <Text style={styles.mono}>{screen.view.payout}</Text>
          <Text style={styles.label}>주문</Text>
          <Text style={styles.mono}>{screen.view.orderShort}</Text>
          <Text style={styles.cta}>기기 버튼을 눌러 승인하세요</Text>
        </>
      )}
      {screen.kind === 'limit' && (
        <>
          <Text style={styles.title}>한도 변경</Text>
          <Text style={styles.label}>1회 한도</Text>
          <Text style={styles.value}>{screen.view.perPayment}</Text>
          <Text style={styles.label}>하루 한도</Text>
          <Text style={styles.value}>{screen.view.daily}</Text>
          <Text style={styles.cta}>기기 버튼으로 PIN을 입력한 뒤 승인 버튼을 누르세요</Text>
        </>
      )}
      {screen.kind === 'result' && (
        <>
          <Text style={styles.title}>{OUTCOME_TEXT[screen.outcome] ?? screen.outcome}</Text>
          {screen.view && <Text style={styles.amount}>{screen.view.amount}</Text>}
          {screen.view && <Text style={styles.body}>{screen.view.merchantName}</Text>}
          {screen.reason && <Text style={styles.body}>{screen.reason}</Text>}
        </>
      )}
    </View>
  );
}

function Button({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <Pressable style={[styles.button, disabled && styles.disabled]} onPress={onPress} disabled={disabled}>
      <Text style={styles.buttonText}>{label}</Text>
    </Pressable>
  );
}

export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="dark-content" />
      <SafeAreaView style={styles.root}>
        <Renter />
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, padding: 24, backgroundColor: '#fafafa' },
  title: { fontSize: 26, fontWeight: '600', color: '#111', marginBottom: 12 },
  body: { fontSize: 17, color: '#333', marginBottom: 12 },
  hint: { fontSize: 15, color: '#666', marginTop: 12 },
  label: { fontSize: 14, color: '#666', marginTop: 12 },
  value: { fontSize: 20, color: '#111' },
  amount: { fontSize: 36, fontWeight: '600', color: '#111' },
  mono: { fontSize: 15, color: '#111', fontFamily: 'monospace' },
  cta: { fontSize: 22, fontWeight: '600', color: '#1b4fa0', marginTop: 28 },
  error: { fontSize: 14, color: '#b00020', marginBottom: 12 },
  input: { fontSize: 15, borderBottomWidth: 1, borderColor: '#333', paddingVertical: 8, marginBottom: 16, color: '#111' },
  button: { backgroundColor: '#111', paddingVertical: 16, borderRadius: 8, alignItems: 'center', marginTop: 8 },
  disabled: { backgroundColor: '#999' },
  buttonText: { color: '#fff', fontSize: 18, fontWeight: '600' },
});
