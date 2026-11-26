import React from 'react';
import {StyleSheet, Text, View} from 'react-native';
import type {CommerceOrderState} from '@nu54/protocol';

/** Whether the store takes no new orders now: paused or a closed day. Busy still takes them. */
export const storeStopped = (state?: CommerceOrderState) => state?.mode === 'paused' || state?.mode === 'closed';

/** What the store said about taking orders, above its menu. refused: a payment was just refused because of it. */
export function StoreNotice({state, refused, checkedAt}: {state?: CommerceOrderState; refused?: boolean; checkedAt?: string}) {
  const kitchen = state?.autoKitchen
    ? <Text style={s.small}>시험 매장 · 자동 주방: 주방 단계를 자동으로 기록하고, 만든 것을 사람이 확인하지 않습니다. 받은 것이 다르면 받은 뒤 이의를 낼 수 있습니다.</Text>
    : null;
  if (!state || state.mode === 'open') return kitchen;
  return <View style={s.gap}>{kitchen}<Notice state={state} refused={refused} checkedAt={checkedAt} /></View>;
}

function Notice({state, refused, checkedAt}: {state: CommerceOrderState; refused?: boolean; checkedAt?: string}) {
  if (state.mode === 'busy')
    return (
      <View accessibilityRole="summary" style={s.box}>
        <Text style={s.title}>{`지금은 약 ${state.busyMinutes}분 걸립니다`}</Text>
        <Text style={s.text}>매장이 알린 시간입니다. 주문은 받습니다.</Text>
      </View>
    );
  return (
    <View style={s.gap}>
      <View accessibilityRole="alert" style={s.box}>
        <Text style={s.title}>{state.mode === 'paused' ? '매장이 주문을 잠시 멈췄습니다' : '오늘은 매장이 쉬는 날입니다'}</Text>
        <Text style={s.text}>
          {state.mode === 'paused'
            ? state.resumeAt ? `${state.resumeAt}쯤 다시 받는다고 알렸습니다.` : '다시 받을 때까지 주문할 수 없습니다.'
            : '다음 날부터 다시 받습니다.'}
        </Text>
      </View>
      {refused && <Text style={s.ok}>결제하지 않았습니다. 돈은 나가지 않았고, 담은 것은 그대로 두었습니다.</Text>}
      {checkedAt && <Text style={s.small}>{`마지막 확인 ${checkedAt}`}</Text>}
    </View>
  );
}

const s = StyleSheet.create({
  gap: {gap: 8},
  box: {backgroundColor: '#f6e3bf', borderRadius: 14, padding: 14, gap: 4},
  title: {fontSize: 17, fontWeight: '800', color: '#5a3b05'},
  text: {fontSize: 15, lineHeight: 22, color: '#5a3b05'},
  ok: {fontSize: 15, lineHeight: 22, color: '#174b38', backgroundColor: '#e3f0e6', borderRadius: 12, padding: 12},
  small: {fontSize: 13, color: '#4f655a'},
});
