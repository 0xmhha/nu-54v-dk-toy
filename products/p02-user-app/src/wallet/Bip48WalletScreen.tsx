import {inputPlaceholder} from '../ui/theme.ts';
import {useEffect,useRef,useState} from 'react';
import {AppState,Keyboard,Pressable, Share, StyleSheet, Text, TextInput, View} from 'react-native';
import {bytesToHex,toBase64} from '@nu54/protocol';
import {loadBip48Policy, prepareBip48, readBip48Import, type Bip48Policy, type Bip48Scope,type StoredBip48} from './bip48Wallet.ts';
import {verifyChainAuthority,checkChainNetwork,loopbackChainRpc,storedChainRpc} from './chainTransfer.ts';
import type {OwnerSession} from './ownerSession.ts';
import type {PersonalMetadata} from './profile.ts';

import type {CommissionedChain} from './commissionedChainProfiles.ts';
import type {StoredChain} from './chainTransfer.ts';

type Props = {
  pickFile?:()=>Promise<string>;
  accountIndex?:number;
  registered?:CommissionedChain;
  token?:string;
  verifyRegistered?:(scope:Pick<StoredChain,'chain'|'network'|'anchor'|'endpoint'>,key:Uint8Array,revision:bigint)=>void;
  saved?:StoredBip48;
  persist?:(wire:string,expires:number,sign:()=>Promise<StoredBip48>)=>Promise<void>;
  verified: PersonalMetadata;
  found: {address: string; name: string}[];
  scan: () => void;
  back: () => void;
  busy: boolean;
  work: (operation: () => Promise<void>, failure: string | ((error: unknown) => string)) => void;
  session: <T>(handle: string, action: (session: OwnerSession) => Promise<T>) => Promise<T>;
  /** `onBoard` marks a prompt shown while the request waits on the independent display and the board. */
  prompt: (message: string, onBoard?: boolean) => void;
  rpc: (endpoint: string, packet: string) => Promise<string>;
  t: (message: string) => string;
};

function Button({text, run, disabled = false}: {text: string; run: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={text} accessibilityState={{disabled}}
    disabled={disabled} onPress={run} style={[styles.button, disabled && styles.disabled]}>
    <Text style={styles.buttonLabel}>{text}</Text></Pressable>;
}

export function Bip48WalletScreen({verified, found, scan, back, busy, work, session, prompt, rpc, t,pickFile,accountIndex=0,registered,token,verifyRegistered,saved,persist}: Props) {
  const [endpoint, setEndpoint] = useState('https://127.0.0.1:8443/bitcoin');
  const [anchor, setAnchor] = useState('');
  const [account, setAccount] = useState(String(accountIndex+1));
  const [participant, setParticipant] = useState('');
  const [selection, setSelected] = useState<{device:string; policy:Bip48Policy; ready:boolean; scope:number}>();
  const [recipient, setRecipient] = useState('');
  const [amount, setAmount] = useState('');
  const [alternative, setAlternative] = useState(false);
  const [counterpart, setCounterpart] = useState('');
  const [importedDocument, setImportedDocument] = useState(false);
  const [settlement,setSettlement]=useState<string>();
  const [preview,setPreview] = useState<Awaited<ReturnType<typeof prepareBip48>>>();
  const epoch=useRef({revision:0,document:0,scope:0});
  useEffect(()=>{
    const lifecycle=epoch.current;
    const listener=AppState.addEventListener('change',state=>{
      if (state==='active') return;
      lifecycle.revision++;
      setPreview(undefined);
    });
    return ()=>{lifecycle.revision++;lifecycle.document++;lifecycle.scope++;listener.remove();};
  },[]);
  const network=registered?2:1;
  const scopeAnchor=registered?.anchor??anchor.trim();
  const route={chain:1 as const,network:network as 1|2,endpoint:registered?.rpc??endpoint.trim(),anchor:scopeAnchor};
  const scopeKey=JSON.stringify([verified.boardFingerprint,verified.generation,verified.address,route.network,route.anchor,route.endpoint,account]);
  const latestScope=useRef(scopeKey);
  if (latestScope.current!==scopeKey) {
    latestScope.current=scopeKey;
    epoch.current.scope++;
    epoch.current.revision++;
    epoch.current.document++;
  }
  const scopeRevision=epoch.current.scope;
  const selected=selection?.scope===scopeRevision ? selection : undefined;
  const exchange = () => registered ? storedChainRpc(route,rpc,token) : loopbackChainRpc(route.endpoint,rpc);
  const savedResult=saved?.board===verified.boardFingerprint && saved.generation===verified.generation &&
    saved.network===network && saved.account===Number(account)-1 ? saved : undefined;
  const shown=savedResult;
  const failure = (error: unknown) => {
    const reason = error instanceof Error ? error.message : '';
    if (reason === 'CHAIN_USER_REFUSED') return t('보드에서 요청을 거절했습니다.');
    if (reason === 'BIP48_THRESHOLD_ALREADY_PRESENT') return t('이미 서명 2개가 있습니다. 독립 지갑에서 확정 상태를 확인하세요.');
    return t('공동서명 정책이나 거래를 확인하지 못했습니다.');
  };

  function discardReview() {
    epoch.current.revision++;
    setPreview(undefined);
  }

  function clearImport() {
    epoch.current.document++;
    discardReview();
    setCounterpart('');
    setImportedDocument(false);
  }

  function chooseDocument() {
    if (!pickFile || busy || !selected?.ready) return;
    work(async()=>{
      clearImport();
      const document=epoch.current.document;
      const current=()=>epoch.current.document===document && epoch.current.scope===scopeRevision;
      Keyboard.dismiss();
      let content:string;
      try {content=await pickFile();}
      catch (error) {if (current()) throw error;return;}
      if (!current()) return;
      setCounterpart(readBip48Import(content));
      setImportedDocument(true);
    },error=>{
      if (error && typeof error==='object' && 'code' in error && error.code==='FILE_CANCELLED')
        return t('PSBT 파일 선택을 취소했습니다.');
      return failure(error);
    });
  }

  async function context(active: OwnerSession) {
    const generation = BigInt(verified.generation);
    await active.verifiedEvmAddress(verified.address, generation);
    const authority = await active.evmAuthority();
    if (registered) {
      if (!verifyRegistered) throw new Error('BIP48_PROFILE');
      verifyRegistered(route,authority.key,authority.revision);
      await checkChainNetwork(exchange(),{chain:1,network,account:Number(account)-1},scopeAnchor);
    }
    if (!/^(?:[1-9]|[1-9][0-9]|100)$/.test(account)) throw new Error('BIP48_ACCOUNT');
    const scope: Bip48Scope = {board: verified.boardFingerprint, generation: String(generation),
      leaseRevision:String(authority.revision),anchor:scopeAnchor,network,account:Number(account)-1};
    return {authority, scope};
  }

  function load(device: string) {
    work(async () => {
      setSelected(undefined);
      clearImport();
      await session(device, async active => {
        const {scope} = await context(active);
        const policy = await loadBip48Policy(exchange(), scope);
        const ready=await active.bip48Status(policy);
        if (epoch.current.scope===scopeRevision) setSelected({device,policy,ready,scope:scopeRevision});
      });
    }, failure);
  }

  function exportParticipant(device: string) {
    work(async () => {
      setParticipant('');
      if (!/^(?:[1-9]|[1-9][0-9]|100)$/.test(account)) throw new Error('BIP48_ACCOUNT');
      const key = await session(device, async active => {
        await context(active);
        return active.exportBip48Participant(Number(account)-1,
          () => prompt(t('독립 화면에서 공개 공동서명 키 내보내기를 확인하세요.'), true),network);
      });
      setParticipant(key);
    }, failure);
  }

  function register() {
    if (!selected) return;
    work(async () => {
      await session(selected.device, async active => {
        const {scope} = await context(active);
        const current = await loadBip48Policy(exchange(), scope);
        if (bytesToHex(current.hash, false) !== bytesToHex(selected.policy.hash, false))
          throw new Error('BIP48_POLICY_MISMATCH');
        await active.registerBip48(current, () => prompt(t('독립 화면에서 세 공동서명 키와 2-of-3 정책을 확인하세요.'), true));
        setSelected({...selected, ready: true});
      });
    }, failure);
  }

  function review() {
    if (!selected?.ready || shown) return;
    work(async()=>{
      const revision=++epoch.current.revision;
      await session(selected.device,async active=>{
        const {authority,scope}=await context(active);
        if (!await active.bip48Status(selected.policy)) throw new Error('BIP48_REGISTRATION_REQUIRED');
        const next=await prepareBip48(exchange(),scope,selected.policy,authority.key,{
          recipient:recipient.trim(),amount:amount.trim(),counterpart:alternative?readBip48Import(counterpart):undefined,
        });
        if (revision===epoch.current.revision) setPreview(next);
      });
    },failure);
  }

  function sign() {
    if (!preview || !selected?.ready || shown) return;
    const progress=preview, policy=selected.policy, revision=epoch.current.revision;
    work(async()=>{
      await session(selected.device,async active=>{
        const {authority,scope}=await context(active);
        verifyChainAuthority(progress.grant,authority.key,scope.board,BigInt(scope.generation),authority.revision,scope.anchor);
        if (revision!==epoch.current.revision || progress.grant.expires<=Math.floor(Date.now()/1000)) throw new Error('BIP48_REVIEW_EXPIRED');
        const approve=async():Promise<StoredBip48>=>{
          const signed=await active.signBip48(policy,progress.grant,
            ()=>prompt(t('독립 화면에서 모든 송금 출력과 수수료를 확인한 뒤 보드에서 승인하세요.'), true));
          if (signed.count!==1 && signed.count!==2) throw new Error('BIP48_SIGNATURE');
          return {...signed,count:signed.count,network,account:policy.account,board:verified.boardFingerprint,
            generation:verified.generation,policy:bytesToHex(policy.hash,false),original:toBase64(progress.grant.raw),id:progress.id};
        };
        if (!persist) throw new Error('BIP48_STORAGE');
        await persist(bytesToHex(progress.grant.body,false),progress.grant.expires,async()=>{
          const receipt=await approve();
          return receipt;
        });
        setPreview(undefined);
      });
    },error=>{setPreview(undefined);return failure(error);});
  }

  return <View>
    <Text accessibilityRole="header" style={styles.label}>{t('Bitcoin 공동서명')} · 2 of 3</Text>
    {registered ? <Text style={styles.label}>Bitcoin Signet · {registered.anchor}</Text> : <>
    <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} value={endpoint} editable={!busy} maxLength={512}
      autoCorrect={false} autoCapitalize="none" accessibilityLabel={t('로컬 HTTPS RPC 주소')}
      onChangeText={value => {setEndpoint(value);setSelected(undefined);clearImport();}} />
    <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} value={anchor} editable={!busy} maxLength={64}
      autoCorrect={false} autoCapitalize="none" accessibilityLabel={t('네트워크 식별자')} placeholder="Regtest genesis hash"
      onChangeText={value => {setAnchor(value);setSelected(undefined);clearImport();}} />
    </>}
    <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} value={account} editable={!busy && !registered} maxLength={3} keyboardType="number-pad"
      accessibilityLabel={t('공동서명 계정 번호')} onChangeText={value => {setAccount(value);setParticipant('');setSelected(undefined);clearImport();}} />
    <Button text={t('개인 기기 찾기')} disabled={busy} run={scan} />
    {found.map(({name, address}) => <Button key={'export-' + address} text={t('공개 공동서명 키 내보내기') + ' · ' + name}
      disabled={busy} run={() => exportParticipant(address)} />)}
    {!!participant && <Text selectable style={styles.label}>{participant}</Text>}
    {found.map(({name, address}) => <Button key={address} text={t('공동서명 지갑 확인') + ' · ' + name}
      disabled={busy || !scopeAnchor} run={() => load(address)} />)}
    {selected && !shown && <>
      <Text selectable style={styles.label}>{t('등록 정책 지문')} {bytesToHex(selected.policy.hash, false)}</Text>
      <Text selectable style={styles.label}>{selected.policy.descriptors.join('\n')}</Text>
      <Text style={styles.label}>{t('공동서명 지문')} {[0,1,2].map(slot=>bytesToHex(selected.policy.bytes.subarray(17+slot*69,21+slot*69),false)).join(' · ')}</Text>
      {!selected.ready && <Button text={t('보드에 공동서명 정책 등록')} disabled={busy} run={register} />}
      {selected.ready && <>
        <Button text={t(alternative ? '새 거래 만들기' : '공동서명 PSBT 가져오기')} disabled={busy}
          run={() => {setAlternative(!alternative);clearImport();}} />
        {alternative ?
          <>
            <Text style={styles.label}>{t('PSBTv2 원본 최대 1,780바이트 · 부분서명 결과 최대 1,998바이트')}</Text>
            {pickFile && <Button text={t('PSBT 파일 또는 QR 이미지 선택')} disabled={busy} run={chooseDocument} />}
            {importedDocument ? <>
              <Text style={styles.label}>{t('PSBT 파일을 불러왔습니다.')}</Text>
              <Button text={t('PSBT 직접 입력')} disabled={busy} run={clearImport} />
            </> : <TextInput placeholderTextColor={inputPlaceholder} multiline style={[styles.input,styles.psbtInput]} value={counterpart} editable={!busy} maxLength={2376}
              autoCorrect={false} autoCapitalize="none" onChangeText={value=>{epoch.current.document++;setCounterpart(value);discardReview();}} accessibilityLabel="PSBT" />}
          </> :
          <>
            <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} value={recipient} editable={!busy} maxLength={85} autoCapitalize="none"
              accessibilityLabel={t('수취인 주소')} placeholder={t('수취인 주소')} onChangeText={value=>{setRecipient(value);discardReview();}} />
            <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} value={amount} editable={!busy} maxLength={20} keyboardType="decimal-pad"
              accessibilityLabel={t('보낼 금액')} placeholder={t('보낼 금액')} onChangeText={value=>{setAmount(value);discardReview();}} />
          </>}
        <Button text={t('PSBT 원문 검토')} disabled={busy || (alternative ? !counterpart : !recipient || !amount)} run={review} />
        {preview && <>
          <Text style={styles.label}>{t('거래 원문 검토')} · {preview.id}</Text>
          <Text style={styles.label}>{t('입력 합계')} {preview.inputTotal} sat · {t('출력 합계')} {preview.outputTotal} sat</Text>
          <Text style={styles.label}>{t('수수료')} {preview.fee} sat · {t('수수료율 하한')} {preview.minimumFeeRate} sat/vB</Text>
          <Text style={styles.label}>{t('최대 서명 거래 크기')} {preview.maximumVsize} vB</Text>
          <Text style={styles.label}>2-of-3 · SIGHASH_ALL · {preview.fingerprints.join(' · ')}</Text>
          {preview.inputs.map(input=><Text key={input.outpoint} selectable style={styles.label}>
            {t('입력 UTXO')} {input.outpoint} · {input.amount} sat{'\n'}scriptPubKey {input.script}
          </Text>)}
          {preview.outputs.map((output,index)=><Text key={index} style={styles.label} selectable>
            {output.change?t('자기 거스름돈'):t('외부 출력')} · {output.address} · {output.satoshis} sat{'\n'}scriptPubKey {output.script}{output.path ? '\n' + output.path : ''}
          </Text>)}
          <Button text={t('거절하고 초안으로')} disabled={busy} run={discardReview} />
          <Button text={t('보드에 부분 서명 요청')} disabled={busy} run={sign} />
        </>}
      </>}
    </>}
        {shown && <>
          {savedResult && <Text selectable style={styles.label}>{savedResult.id}</Text>}
          <Button text={t('원 거래 상태 재조회')} disabled={busy || !scopeAnchor} run={()=>work(async()=>{
            const active=exchange();
            await checkChainNetwork(active,{chain:1,network,account:shown.account},scopeAnchor);
            const result=await active('getrawtransaction',[shown.id,true]);
            if (result===null) {setSettlement(t('선택한 원장에서 원 거래를 기다리는 중입니다.'));return;}
            if (!result || typeof result!=='object' || !('txid' in result) || result.txid!==shown.id ||
                !('confirmations' in result) || typeof result.confirmations!=='number') throw new Error('BIP48_TX_ID');
            setSettlement(t(result.confirmations>=6 ? '원 거래가 확정되었습니다.' : '선택한 원장에서 원 거래를 기다리는 중입니다.'));
          },failure)} />
          {settlement && <Text style={styles.label}>{settlement}</Text>}
          <Text style={styles.label}>{shown.count === 2 ? t('서명 2개. 독립 지갑에서 결합과 확정 상태를 확인하세요.') :
            t('서명 1개. 공동서명자 한 명의 서명이 더 필요합니다.')}</Text>
          <Button text={t('서명된 PSBT 공유')} disabled={busy} run={() => work(async () => {
            await Share.share({message: shown.psbt});
          }, t('PSBT를 공유하지 못했습니다.'))} />
        </>}
    <Button text={t('받기와 보내기로 돌아가기')} disabled={busy} run={()=>{clearImport();back();}} />
  </View>;
}

const styles = StyleSheet.create({
  button: {backgroundColor: '#19446b', borderRadius: 7, margin: 4, padding: 11},
  disabled: {opacity: 0.55},
  buttonLabel: {color: '#fff', fontWeight: '600'},
  input: {borderColor: '#71839a', borderWidth: 1, backgroundColor: '#fff', color: '#0b1730', margin: 4, padding: 9},
  psbtInput: {height: 88, textAlignVertical: 'top'},
  label: {color: '#0b1730', padding: 7},
});
