import {nonEvmAssetKey, scopedNonEvmAssets} from './nonEvmAssetCatalog.ts';
import {unknownConnectivity, type ReadProof, type WalletConnectivity} from './walletConnectivity.ts';
import React from 'react';
import {WalletEducationRateCard} from './WalletEducationRateCard.tsx';
import {educationRate, stableTestUsdc} from './walletEducationRate.ts';
import {Pressable, StyleSheet, Text, View} from 'react-native';
import {accountPreference, assetsForAccount} from './walletCatalog.ts';
import {defaultReadState, formatReadAmount, observationScope, readProfiles, type ReadProfileState} from './readProfiles.ts';

type Owner = {boardFingerprint:string; generation:number; address:string};
export function WalletHomeSummary({owner, state = defaultReadState, language, active, connectivity = unknownConnectivity, proof, onHide, onAccounts, onReceive}: {
  owner:Owner; state?:ReadProfileState; language:'ko'|'en'; active:boolean; connectivity?:WalletConnectivity; proof?:ReadProof;
  onHide:()=>void; onAccounts:()=>void; onReceive:()=>void;
}) {
  const ko = language === 'ko';
  const selected = state.selection;
  const account = accountPreference(state.accounts, owner, selected);
  const nativeAsset = !selected.asset || /^0x0{40}$/.test(selected.asset);
  const asset = nativeAsset ? undefined : assetsForAccount(state.assets, owner, selected).find(item => item.contract === selected.asset && !item.hidden);
  const token = scopedNonEvmAssets(state.nonEvmAssets,owner,selected).find(item => !item.hidden && nonEvmAssetKey(item) === selected.asset);
  const available = !state.disabled?.includes(selected.id) && !account?.hidden && (nativeAsset || !!asset || !!token);
  const scope = observationScope(owner.boardFingerprint, owner.generation, selected);
  const snapshot = available ? state.observations.find(item => item.scope === scope &&
    (!('evm' in readProfiles[selected.id]) || item.address === owner.address) && (!asset || item.decimals === asset.decimals) &&
    (token?.id !== 'sol' || item.decimals === token.decimals)) : undefined;
  const hidden = selected.hidden || !active;
  const rate = hidden ? undefined : educationRate(state,owner);
  const unit = selected.id === 'stable' && asset?.contract === stableTestUsdc && asset.decimals === 6 ? 'TestUSDC' : snapshot?.unit;
  const rechecked = connectivity.link === 'online' && proof?.revision === connectivity.revision &&
    proof.scope === scope && proof.endpoint === state.endpoints[selected.id] && proof.time === snapshot?.time;
  const linkLabel = connectivity.link === 'offline' ? (ko ? '오프라인 · 마지막 조회' : 'Offline · last observation') :
    rechecked ? (ko ? '연결됨 · 원장 재확인 완료' : 'Connected · ledger verified') :
      connectivity.link === 'online' ? (ko ? '연결됨 · 원장 재확인 필요' : 'Connected · ledger check required') :
        (ko ? '마지막 조회 · 현재 연결 미확인' : 'Last observation · connection not checked');
  const button = (label:string, run:()=>void) => <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={run} style={styles.button}><Text style={styles.buttonText}>{label}</Text></Pressable>;
  return <View style={styles.card}>
    <Text accessibilityRole="header" style={styles.heading}>{ko ? '내 자산' : 'My assets'}</Text>
    <Text style={styles.text}>{ko ? '개인 소유 프로필 · 세대 ' : 'Personal owner profile · generation '}{owner.generation}</Text>
    <Text style={styles.text}>{readProfiles[selected.id].label} · {account?.name ?? ((ko ? '계정 ' : 'Account ') + (selected.account + 1))}</Text>
    {!available ? <Text style={styles.text}>{ko ? '선택한 계정 또는 자산을 사용할 수 없습니다.' : 'The selected account or asset is unavailable.'}</Text> :
      hidden ? <Text style={styles.text}>{ko ? '잔액 숨김' : 'Balance hidden'}</Text> :
      !snapshot ? <Text style={styles.text}>{ko ? '조회 기록 없음' : 'No balance observation'}</Text> :
      snapshot.inactive ? <Text style={styles.text}>{ko ? '계정이 활성화되지 않았습니다.' : 'Account is inactive.'}</Text> :
      snapshot.unconfigured ? <Text style={styles.text}>{ko ? '설정된 발행 자산 신뢰선 없음' : 'No configured trust line for this issued asset'}</Text> :
      <Text style={styles.balance}>{snapshot.issued ?? formatReadAmount(snapshot.amount!, snapshot.decimals)} {unit}</Text>}
    {active && <Text accessibilityLiveRegion="polite" style={styles.text}>{linkLabel}</Text>}
    {active && connectivity.link === 'offline' && <Text style={styles.text}>
      {ko ? '현재 지급 가능액 · 확인 대기' : 'Available to spend · verification pending'}
    </Text>}
    {available && snapshot && !hidden && <Text style={styles.text}>{snapshot.point}{'\n'}{new Date(snapshot.time).toLocaleString(language === 'ko' ? 'ko-KR' : 'en-US')}</Text>}
    {rate && <WalletEducationRateCard key={rate.scope} rate={rate} stale={connectivity.link !== 'online'} language={language} />}
    {button(selected.hidden ? (ko ? '잔액 표시' : 'Show balance') : (ko ? '잔액 숨기기' : 'Hide balance'), onHide)}
    {button(ko ? '계정 및 자산 선택' : 'Choose account and asset', onAccounts)}
    {button(ko ? '받기' : 'Receive', onReceive)}
  </View>;
}
const styles = StyleSheet.create({card:{padding:16, borderWidth:1, borderColor:'#777', borderRadius:12, gap:8}, heading:{fontSize:22,fontWeight:'700',color:'#111'},text:{fontSize:16,color:'#111'},balance:{fontSize:26,fontWeight:'700',color:'#111'},button:{padding:12,backgroundColor:'#123f6b',borderRadius:8},buttonText:{color:'#fff',fontSize:16,fontWeight:'600'}});
