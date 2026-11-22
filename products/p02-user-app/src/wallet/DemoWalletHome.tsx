import React, {useMemo, useState} from 'react';
import {ScrollView, StyleSheet, Text, View} from 'react-native';
import {Brand, KitButton, Section, TabBar} from '../ui/kit.tsx';
import {color, radius, font} from '../ui/theme.ts';
import {defaultReadState, observationScope, type ReadProfileState} from './readProfiles.ts';
import {WalletHomeSummary} from './WalletHomeSummary.tsx';

// Fixture-only wallet home for design review on a QA build (OwnerWallet.demoWallet()). It never
// reads or writes the stored profile, never connects to a board and every action is disabled, so
// nothing here can approve, sign or send. The banner stays on screen the whole time.
const demoOwner = {boardFingerprint: 'de'.repeat(32), generation: 1, address: '0x' + 'de'.repeat(20)};

export function demoReadState(now: number): ReadProfileState {
  const selection = defaultReadState.selection;
  return {...defaultReadState, observations: [{
    address: demoOwner.address, scope: observationScope(demoOwner.boardFingerprint, demoOwner.generation, selection),
    time: now, point: '시연 데이터', amount: '42000000', decimals: 6, unit: 'TestUSDC',
  }]};
}

export function DemoWalletHome({t, close}: {t: (ko: string) => string; close: () => void}) {
  const [hidden, setHidden] = useState(false);
  const base = useMemo(() => demoReadState(Date.now()), []);
  const state = {...base, selection: {...base.selection, hidden}};
  const off = () => {};
  const group = (title: string, labels: string[]) => <Section title={t(title)}>
    {labels.map(label => <KitButton key={label} label={t(label)} run={off} disabled variant="row" />)}
  </Section>;
  return <View style={styles.screen} testID="demo-wallet-home">
    <ScrollView contentContainerStyle={styles.content}>
      <View style={styles.banner} accessibilityRole="alert">
        <Text style={styles.bannerTitle}>{t('시연용 화면')}</Text>
        <Text style={styles.bannerText}>{t('실제 보드 확인 없이 만든 예시입니다. 잔액과 주소는 가짜이며 승인·서명·전송은 모두 꺼져 있습니다.')}</Text>
      </View>
      <Brand chip={{label: t('시연용'), tone: 'wait'}} />
      <Text accessibilityRole="header" style={styles.title}>{t('개인 지갑')}</Text>
      <WalletHomeSummary owner={demoOwner} state={state} language="ko" active
        onHide={() => setHidden(value => !value)} onAccounts={off} onReceive={off} inert />
      <View style={styles.pair}>
        <View style={styles.pairItem}><KitButton label={t('다른 체인 받기')} run={off} disabled variant="secondary" /></View>
        <View style={styles.pairItem}><KitButton label={t('다른 체인 보내기')} run={off} disabled variant="secondary" /></View>
      </View>
      {group('자산과 네트워크', ['6개 체인 · 읽기 전용', 'EVM 수신', '네트워크 및 자산 설정', '개인 전송', '지급 기록 관리'])}
      {group('앱 연결과 권한', ['외부 DApp 연결', 'DApp 둘러보기', '온체인 토큰 권한'])}
      {group('보안과 복구', ['독립 백업 시작', '원래 보드에서 분실한 휴대전화 교체', '휴대전화 분실 및 지갑 복구', '펌웨어 적용 증거 확인'])}
      <KitButton label={t('시연 화면 닫기')} run={close} variant="primary" />
    </ScrollView>
    <TabBar current="wallet" disabled tabs={[
      {key: 'wallet', label: t('지갑'), run: off}, {key: 'pay', label: t('결제'), run: off},
      {key: 'explore', label: t('둘러보기'), run: off}, {key: 'settings', label: t('설정'), run: off},
    ]} />
  </View>;
}

const styles = StyleSheet.create({
  screen: {flex: 1, gap: 12},
  content: {gap: 12, paddingBottom: 24},
  banner: {backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12, gap: 4},
  bannerTitle: {fontFamily: font, fontSize: 15, fontWeight: '800', color: color.amber},
  bannerText: {fontFamily: font, fontSize: 13, lineHeight: 19, color: '#5a3b05'},
  title: {fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink},
  pair: {flexDirection: 'row', gap: 10},
  pairItem: {flex: 1},
});
