import {inputPlaceholder} from '../ui/theme.ts';
import {useState} from 'react';
import {Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import {nonEvmAssetKey, registerNonEvmAsset, reviewNonEvmAsset, scopedNonEvmAssets,
  type NonEvmAssetReview} from './nonEvmAssetCatalog.ts';
import {readProfiles, verifyReadNetwork, walletReadRpc, type ReadProfileState} from './readProfiles.ts';
import type {PersonalMetadata} from './profile.ts';
import {assetSearchMatches} from './walletEducationRate.ts';

type Props = {
  owner: PersonalMetadata; value: ReadProfileState; busy: boolean; enabled: boolean; save: (value: ReadProfileState) => Promise<void>;
  language?: 'ko' | 'en';
  rpc: (endpoint: string, packet: string) => Promise<string>;
  work: (operation: () => Promise<void>, failure: string | ((error:unknown)=>string)) => void; t: (message: string) => string;
};

export function WalletNonEvmAssets({owner,value,busy,enabled,save,rpc,work,t,language='ko'}:Props) {
  const [identity,setIdentity] = useState('');
  const [extra,setExtra] = useState('');
  const [candidate,setCandidate] = useState<NonEvmAssetReview>();
  const [group,setGroup] = useState<'all'|'hidden'|'favorite'>('all');
  const [search,setSearch] = useState('');
  const {selection} = value;
  if (selection.id !== 'sol' && selection.id !== 'xrp') return null;
  const endpoint = value.endpoints[selection.id];
  const profile = readProfiles[selection.id];
  const solana = selection.id === 'sol';
  const reviewed = candidate?.board === owner.boardFingerprint && candidate.generation === owner.generation &&
    candidate.account === selection.account && candidate.id === selection.id && candidate.endpoint === endpoint ? candidate : undefined;
  const listed = scopedNonEvmAssets(value.nonEvmAssets,owner,selection).filter(item =>
    (group === 'hidden' ? item.hidden : !item.hidden && (group === 'all' || item.favorite)) &&
    (item.id === 'sol' ? assetSearchMatches(search,item.mint,profile.label) :
      assetSearchMatches(search,item.currency,item.issuer,profile.label)));
  const button = (label:string, run:()=>void, disabled=busy) => <Pressable accessibilityRole="button"
    accessibilityLabel={label} accessibilityState={{disabled}} disabled={disabled} onPress={run}
    style={[styles.button, disabled && styles.disabled]}><Text style={styles.buttonText}>{label}</Text></Pressable>;
  const changeSelection = (asset?:string) => work(async () => {
    await save({...value,selection:{...selection,asset}});
    setCandidate(undefined);
  },t('자산 표시를 저장하지 못했습니다.'));
  return <View>
    <Text accessibilityRole="header" style={styles.text}>{t(solana ? 'Solana 토큰 관리' : 'XRPL 발행 자산 관리')}</Text>
    <TextInput placeholderTextColor={inputPlaceholder} accessibilityLabel={t(solana ? 'SPL 민트 주소' : 'XRPL 통화 코드')} style={styles.input}
      value={identity} maxLength={44} editable={!busy} autoCapitalize="none" autoCorrect={false}
      onChangeText={text => {setIdentity(text); setCandidate(undefined);}} />
    <TextInput placeholderTextColor={inputPlaceholder} accessibilityLabel={t(solana ? '예상 소수 자릿수' : 'XRPL 발행자 주소')} style={styles.input}
      value={extra} maxLength={solana ? 2 : 35} editable={!busy} autoCapitalize="none" autoCorrect={false}
      onChangeText={text => {setExtra(text); setCandidate(undefined);}} />
    {button(t('토큰 정보 검토'), () => work(async () => {
      if (!endpoint) throw new Error('WALLET_ASSET_SCOPE');
      setCandidate(undefined);
      const chain = walletReadRpc(endpoint,rpc);
      if (!solana) await verifyReadNetwork(chain,'xrp');
      setCandidate(await reviewNonEvmAsset(chain,owner,selection,endpoint,profile.anchor,identity.trim(),extra.trim()));
    }, t('선택한 네트워크에서 토큰을 검증하지 못했습니다.')),busy || !enabled || !endpoint || !identity.trim() || (!solana && !extra.trim()))}
    {reviewed && <>
      <Text selectable style={styles.text}>{profile.label} · {reviewed.id === 'sol' ?
        reviewed.mint + '\nSPL Token · ' + t('소수 자릿수') + ' ' + reviewed.decimals + ' · ' + t('확정 슬롯') + ' ' + reviewed.slot :
        reviewed.currency + '\n' + reviewed.issuer + '\n' + t('검증 원장') + ' ' + reviewed.ledgerIndex + ' · ' + reviewed.ledgerHash}</Text>
      {reviewed.id === 'xrp' && <Text style={styles.text}>{t('발행자 계정을 확인했습니다. 자산 잔액에는 별도의 신뢰선이 필요합니다.')}</Text>}
      {button(t('검증된 토큰 추가'), () => work(async () => {
        await save(registerNonEvmAsset(value,owner,reviewed));
        setCandidate(undefined);
      },t('검증한 토큰을 저장하지 못했습니다.')),busy || !enabled)}
      {button(t('토큰 검토 취소'), () => setCandidate(undefined))}
    </>}
    {button(t('기본 자산 보기'), () => changeSelection(),busy || !selection.asset)}
    {button(t('등록 토큰'),()=>setGroup('all'),busy || group==='all')}
    {button(t('즐겨찾기'),()=>setGroup('favorite'),busy || group==='favorite')}
    {button(t('숨긴 자산'),()=>setGroup('hidden'),busy || group==='hidden')}
    <TextInput placeholderTextColor={inputPlaceholder} accessibilityLabel={language === 'ko' ? '등록 자산 검색' : 'Search registered assets'} style={styles.input}
      value={search} maxLength={64} autoCapitalize="none" autoCorrect={false} onChangeText={setSearch} />
    {search.trim() && !listed.length && <Text style={styles.text}>
      {language === 'ko' ? '일치하는 등록 자산이 없습니다.' : 'No matching registered assets.'}
    </Text>}
    {listed.map((item,index) => {
      const key = nonEvmAssetKey(item);
      const change = (field:'favorite'|'hidden') => work(async () => {
        await save({...value,nonEvmAssets:value.nonEvmAssets?.map(row => row === item ? {...row,[field]:!item[field]} : row),
          selection:field === 'hidden' && !item.hidden && selection.asset === key ? {...selection,asset:undefined} : selection});
      },t('자산 표시를 저장하지 못했습니다.'));
      return <View key={key}>
        <Text selectable style={styles.text}>{item.id === 'sol' ? 'SPL · ' + item.mint + ' · ' + item.decimals :
          item.currency + ' · ' + item.issuer}</Text>
        {button(t('조회 토큰 선택') + ' ' + (index+1),()=>changeSelection(key),busy || item.hidden || !enabled)}
        {button(t(item.favorite?'즐겨찾기 해제':'즐겨찾기에 추가')+' '+(index+1),()=>change('favorite'),busy || item.hidden)}
        {button(t(item.hidden?'목록에 다시 표시':'목록에서 숨기기')+' '+(index+1),()=>change('hidden'))}
      </View>;
    })}
  </View>;
}

const styles = StyleSheet.create({
  text:{color:'#0b1730',padding:7}, input:{borderColor:'#71839a',borderWidth:1,backgroundColor:'#fff',color:'#0b1730',margin:4,padding:9},
  button:{backgroundColor:'#19446b',borderRadius:7,margin:4,padding:11},buttonText:{color:'#fff',fontWeight:'600'},disabled:{opacity:0.55},
});
