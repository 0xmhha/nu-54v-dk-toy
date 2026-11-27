import React from 'react';
import {StyleSheet, Text, View} from 'react-native';
import type {CommerceOrderState} from '@nu54/protocol';

/** Whether the store takes no new orders now: paused or a closed day. Busy still takes them. */
export const storeStopped = (state?: CommerceOrderState) => state?.mode === 'paused' || state?.mode === 'closed';

/**
 * What the store said about taking orders, above the menu. Busy is a band and the menu stays; paused and a closed day
 * take the menu's place. Nothing here moves: the state is read, not announced.
 */
export function StoreNotice({state, ko, cleared}: {state?: CommerceOrderState; ko: boolean; cleared?: boolean}) {
  const kitchen = state?.autoKitchen ? <Text style={s.kitchen}>{ko
    ? '시험 매장 · 자동 주방: 주방 단계를 자동으로 기록합니다'
    : 'Test store · automatic kitchen: the kitchen stages are recorded automatically'}</Text> : null;
  if (!state || state.mode === 'open') return kitchen;
  return <View style={s.stack}>{kitchen}<Notice state={state} ko={ko} cleared={cleared} /></View>;
}

function Notice({state, ko, cleared}: {state: CommerceOrderState; ko: boolean; cleared?: boolean}) {
  if (state.mode === 'busy')
    return (
      <View accessibilityRole="summary" style={s.band}>
        <Text style={s.bandTitle}>{ko ? `지금은 약 ${state.busyMinutes}분 걸립니다` : `Orders take about ${state.busyMinutes} minutes now`}</Text>
        <Text style={s.bandText}>{ko ? '매장이 알린 시간입니다. 주문은 받습니다.' : 'The store says so. Orders are still taken.'}</Text>
      </View>
    );
  const paused = state.mode === 'paused';
  return (
    <View accessibilityRole="alert" style={s.panel}>
      <Text accessibilityRole="header" style={s.panelTitle}>
        {paused ? (ko ? '지금은 주문을 잠시 멈췄습니다' : 'Orders are paused for now') : (ko ? '오늘은 쉬는 날입니다' : 'Closed today')}
      </Text>
      <Text style={s.panelText}>
        {paused
          ? state.resumeAt
            ? (ko ? `매장이 ${state.resumeAt}쯤 다시 받는다고 알렸습니다. 이미 받은 주문은 순서대로 준비합니다.`
                  : `The store says it will take orders again around ${state.resumeAt}. Orders already placed are being made.`)
            : (ko ? '이미 받은 주문은 순서대로 준비합니다.' : 'Orders already placed are being made.')
          : (ko ? '오늘은 매장이 정한 휴무일이라 주문을 받지 않습니다. 다음 날부터 다시 받습니다.'
                : 'The store is closed today and takes no orders. It takes them again from tomorrow.')}
      </Text>
      {cleared && <Text style={s.panelText}>{ko ? '담아 둔 것을 지웠습니다. 돈은 나가지 않았습니다.' : 'Your basket was cleared. Nothing was charged.'}</Text>}
      {paused && <Text style={s.panelText}>{ko ? '휴대전화 주문도 함께 멈춥니다.' : 'Phone orders are paused too.'}</Text>}
    </View>
  );
}

const s = StyleSheet.create({
  stack: {gap: 10},
  kitchen: {alignSelf: 'flex-start', fontSize: 15, fontWeight: '700', color: '#5a3b05', backgroundColor: '#f6e3bf', borderRadius: 10, paddingHorizontal: 12, paddingVertical: 6},
  band: {backgroundColor: '#f6e3bf', borderRadius: 14, paddingVertical: 14, paddingHorizontal: 18, gap: 4},
  bandTitle: {fontSize: 24, fontWeight: '800', color: '#5a3b05'},
  bandText: {fontSize: 17, color: '#5a3b05', lineHeight: 24},
  panel: {backgroundColor: '#f6e3bf', borderRadius: 18, padding: 32, gap: 14},
  panelTitle: {fontSize: 40, lineHeight: 50, fontWeight: '800', color: '#5a3b05', letterSpacing: -1},
  panelText: {fontSize: 20, lineHeight: 30, color: '#5a3b05'},
});
