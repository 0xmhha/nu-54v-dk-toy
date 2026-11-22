import {useEffect, useRef, useState} from 'react';
import {Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import {formatReadAmount, readCustomEvmBalance, readCustomEvmDescriptor, setCustomEvmDescriptor,
  verifyCustomEvmNetwork, walletReadRpc, type CustomEvmDescriptor} from './readProfiles.ts';

type Props = {
  profiles: CustomEvmDescriptor[];
  save: (profiles: CustomEvmDescriptor[]) => Promise<void>;
  readAddress: (handle: string) => Promise<string>;
  found: {address: string; name: string}[];
  discover: () => void;
  exchange: (endpoint: string, packet: string) => Promise<string>;
  work: (operation: () => Promise<void>, failure: string) => void;
  busy: boolean;
  back: () => void;
  t: (message: string) => string;
};

const fields = {
  name: '망 이름', rpc: '검증된 HTTPS RPC 주소', chainId: '체인 ID', genesis: '제네시스 해시',
  explorer: 'HTTPS 탐색기 주소', currency: '통화 단위', decimals: '소수 자릿수',
  feeCap: '최대 수수료 (최소 단위)', confirmations: '확정 수 (0은 finalized)',
} as const;

type Draft = Record<keyof typeof fields, string>;
const empty: Draft = {name: '', chainId: '', genesis: '', rpc: '', explorer: '', currency: '', decimals: '18',
  feeCap: '', confirmations: '0'};

export function WalletCustomNetworkScreen({profiles, save, readAddress, found, discover, exchange, work, busy, back, t}: Props) {
  const [draft, setDraft] = useState<Draft>(empty);
  const [preview, setPreview] = useState<{network: string; address: string; height: string; amount: string; at: number}>();
  const [chosen, setChosen] = useState<string>();
  const [verification, setVerification] = useState<'idle' | 'checking' | 'saving' | 'canceled' | 'failed'>('idle');
  const currentVerification = useRef<{saving:boolean} | undefined>(undefined);
  useEffect(() => () => { currentVerification.current = undefined; }, []);
  const checking = verification === 'checking';
  const saving = verification === 'saving';
  const locked = busy || checking || saving;
  const active = profiles.find(item => item.chainId === chosen);
  const button = (label: string, action: () => void, disabled = locked) => <Pressable accessibilityRole="button"
    accessibilityLabel={label} accessibilityState={{disabled}} disabled={disabled} onPress={action}
    style={[styles.button, disabled && styles.disabled]}><Text style={styles.buttonLabel}>{label}</Text></Pressable>;

  function cancelVerification() {
    if (currentVerification.current?.saving) return;
    currentVerification.current = undefined;
    setVerification('canceled');
  }

  async function verifyAndSave() {
    if (currentVerification.current || busy) return;
    const operation = {saving:false};
    currentVerification.current = operation;
    setVerification('checking');
    try {
      if (!/^(?:0|[1-9][0-9]{0,4})$/.test(draft.confirmations) || !/^[0-9]{1,2}$/.test(draft.decimals))
        throw new Error('WALLET_CUSTOM_SCHEMA');
      const candidate = readCustomEvmDescriptor({...draft, engine: 'evm', decimals: Number(draft.decimals),
        confirmations: Number(draft.confirmations)});
      const next = setCustomEvmDescriptor(profiles, candidate);
      await verifyCustomEvmNetwork(candidate, walletReadRpc(candidate.rpc, (endpoint, packet) => {
        if (currentVerification.current !== operation) throw new Error('WALLET_CUSTOM_CANCELED');
        return exchange(endpoint, packet);
      }));
      if (currentVerification.current !== operation) return;
      operation.saving = true;
      setVerification('saving');
      await save(next);
      if (currentVerification.current !== operation) return;
      setPreview(undefined);
      setChosen(candidate.chainId);
      setVerification('idle');
    } catch {
      if (currentVerification.current === operation) setVerification('failed');
    } finally {
      if (currentVerification.current === operation) currentVerification.current = undefined;
    }
  }

  function refresh(handle: string) {
    if (!active) return;
    work(async () => {
      setPreview(undefined);
      const address = await readAddress(handle);
      const result = await readCustomEvmBalance(active, address, walletReadRpc(active.rpc, exchange));
      setPreview({network: active.chainId, address: result.address, height: result.height,
        amount: formatReadAmount(result.amount, active.decimals) + ' ' + active.currency, at: Date.now()});
    }, t('확정 입금 잔액을 확인하지 못했습니다.'));
  }

  return <View>
    <Text accessibilityRole="header" style={styles.label}>{t('사용자 지정 EVM 망 · 읽기 전용')}</Text>
    {profiles.map(item => <View key={item.chainId}>{button(item.name, () => {
      setDraft({...item, decimals: String(item.decimals), confirmations: String(item.confirmations)});
      setChosen(item.chainId);
      setPreview(undefined);
    })}</View>)}
    {Object.entries(fields).map(([name, label]) => <View key={name}>
      <Text style={styles.label}>{t(label)}</Text>
      <TextInput accessibilityLabel={t(label)} style={styles.input} value={draft[name as keyof Draft]} maxLength={512}
        editable={!locked} autoCorrect={false} autoCapitalize="none"
        onChangeText={value => {setDraft({...draft, [name]: value}); setPreview(undefined);}} />
    </View>)}
    {button(t('지문 확인 후 읽기 전용 RPC 저장'), () => {void verifyAndSave();})}
    {verification !== 'idle' && <Text accessibilityLiveRegion="polite" style={styles.label}>{t({
      checking:'RPC 네트워크 지문 확인 중', saving:'검증된 읽기 전용 RPC 저장 중', canceled:'검증을 취소했습니다.',
      failed:'사용자 지정 네트워크의 지문을 확인하지 못했습니다.',
    }[verification])}</Text>}
    {checking && button(t('검증 취소'), cancelVerification, false)}
    {active && <>
      <Text selectable style={styles.label}>{active.name} · {active.chainId} · {active.genesis}</Text>
      <Text selectable style={styles.label}>{active.explorer}</Text>
      {button(t('개인 기기 찾기'), discover)}
      {found.map(item => <View key={item.address}>{button(t('입금 잔액 다시 확인') + ' · ' + item.name,
        () => refresh(item.address))}</View>)}
    </>}
    {active && preview?.network === active.chainId && <Text selectable style={styles.label}>
      {preview.address} · {preview.amount} · {preview.height} · {new Date(preview.at).toISOString()}
    </Text>}
    {button(t('받기와 보내기로 돌아가기'), () => {
      if (currentVerification.current?.saving) return;
      if (checking) cancelVerification();
      back();
    }, busy || saving)}
  </View>;
}

const styles = StyleSheet.create({
  button: {backgroundColor: '#19446b', borderRadius: 7, margin: 4, padding: 11},
  buttonLabel: {color: '#fff', fontWeight: '600'},
  disabled: {opacity: 0.55},
  label: {color: '#0b1730', padding: 7},
  input: {borderColor: '#71839a', borderWidth: 1, backgroundColor: '#fff', color: '#0b1730', margin: 4, padding: 9},
});
