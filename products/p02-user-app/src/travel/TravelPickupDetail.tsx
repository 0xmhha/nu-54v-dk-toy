import {useCallback, useEffect, useState} from 'react';
import {Pressable, StyleSheet, Text, View} from 'react-native';
import {commerceAmount, formatPickupTime, readPickupCustomer, type CommercePaymentProfile,
  type PickupCustomerOrder, type PickupOrderState} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';

export const pickupStateLabel: Record<PickupOrderState, string> = {
  'waiting-payment': '지급 확인 중',
  'awaiting-acceptance': '지급 확정 · 매장 접수 대기',
  accepted: '매장 접수',
  preparing: '준비 중',
  ready: '픽업 준비 완료',
  collected: '수령 완료',
  unfulfillable: '매장 이행 불가',
  closed: '주문 종료',
};

const refundLabel = {
  'awaiting-request': '재무 검토 대기',
  pending: '환불 지급 확인 중',
  rejected: '재무 검토 결과 반려',
  confirmed: '환불 지급 확정',
};

export function TravelPickupDetail({branch, sale, policy, orderRevision}: {
  branch: string; sale: string; policy?: CommercePaymentProfile; orderRevision: number;
}) {
  const [item, setItem] = useState<PickupCustomerOrder>();
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const refresh = useCallback(() => setAttempt(n => n + 1), []);
  useEffect(() => {
    let active = true;
    setError(false);
    async function read() {
      const raw = await Http!.exchange('customer', JSON.stringify({travel: {operation: 'pickup-owned', branch, sale}}));
      const [value] = readPickupCustomer(raw);
      if (!value || value.saleRef !== sale) throw new Error('Pickup unavailable');
      if (active) setItem(value);
    }
    read().catch(() => {
      if (active) {
        setItem(undefined);
        setError(true);
      }
    });
    return () => { active = false; };
  }, [attempt, branch, orderRevision, sale]);
  return <View style={styles.area}>
    {item && <>
      <Text>픽업 시간 · {formatPickupTime(item.startsAt, item.timezone)}</Text>
      <Text>{pickupStateLabel[item.state]}</Text>
      {item.pickupNo !== undefined && <Text>픽업 번호 {item.pickupNo}</Text>}
      {item.refund && <>
        <Text>{refundLabel[item.refund.state]}</Text>
        {item.refund.amount && <Text>환불 금액 {commerceAmount(item.refund.amount, item.asset, policy)}</Text>}
        {item.refund.receiptRef && <Text selectable>환불 지급 증거 {item.refund.receiptRef}</Text>}
      </>}
    </>}
    {error && <Text accessibilityRole="alert">픽업 또는 환불 상태를 확인할 수 없습니다.</Text>}
    <Pressable accessibilityRole="button" accessibilityLabel="이 주문의 픽업과 환불 상태 다시 조회" onPress={refresh} style={styles.button}>
      <Text>이 주문의 픽업과 환불 상태 다시 조회</Text>
    </Pressable>
  </View>;
}

const styles = StyleSheet.create({
  area: {gap: 5},
  button: {backgroundColor: '#e5ede9', padding: 11, alignItems: 'center', borderRadius: 5},
});
