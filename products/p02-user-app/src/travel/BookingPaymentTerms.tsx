import {Text, View} from 'react-native';
import {commerceAmount, type CommercePaymentProfile} from '@nu54/protocol';

export type TravelTerms = {
  termsVersion: number;
  cancellation: string;
  cancellationDeadline?: string;
  noShow?: string;
  priceVersion?: number;
  chainId?: number;
  asset: string;
  deposit: string;
  balance: string;
};

export type TravelBooking = TravelTerms & {
  id: string;
  slot: string;
  partySize: number;
  startsAt?: string;
  expiresAt: string;
  state: string;
};

export type TravelBookingScope = {branch: string; customer: string; tenant: string};
export type TravelLegKind = 'deposit' | 'balance';
export type TravelBookingPayment = {
  booking: TravelBooking;
  kind: TravelLegKind;
  scope: TravelBookingScope;
};

const bookingTime = (value?: string) => value && Number.isFinite(Date.parse(value))
  ? new Date(value).toLocaleString(undefined, {timeZoneName: 'short'}) : '원예약 기록 없음';

export function BookingPolicy({terms}: {terms: TravelTerms}) {
  return <View>
    <Text>예약 정책 v{terms.termsVersion} · 가격 {terms.priceVersion === undefined ? '기록 없음' : `v${terms.priceVersion}`}</Text>
    <Text>취소 조건 · {terms.cancellation}</Text>
    <Text>취소 기한 · {bookingTime(terms.cancellationDeadline)}</Text>
    <Text>노쇼 · {terms.noShow ?? '원예약 기록 없음'}</Text>
    <Text>예약 네트워크 · {terms.chainId === 8283 ? 'StableNet 테스트넷 8283' : terms.chainId ?? '원예약 기록 없음'}</Text>
  </View>;
}

export function BookingPaymentTerms({payment, policy}: {payment: TravelBookingPayment; policy?: CommercePaymentProfile}) {
  const {booking, kind} = payment;
  const amount = (units: string) => commerceAmount(units, booking.asset, policy);
  return <View>
    <Text accessibilityRole="header">{kind === 'deposit' ? '예약금 지급 검토' : '잔금 지급 검토'}</Text>
    <Text selectable>원예약 {booking.id}</Text>
    <Text>이용 시각 · {bookingTime(booking.startsAt)} · {booking.partySize}명</Text>
    <Text>예약 총액 {amount((BigInt(booking.deposit) + BigInt(booking.balance)).toString())}</Text>
    <Text>{kind === 'balance' ? '기확정 예약금' : '이번 예약금'} {amount(booking.deposit)}</Text>
    <Text>{kind === 'balance' ? '이번 잔금' : '별도 잔금'} {amount(booking.balance)}</Text>
    <BookingPolicy terms={booking} />
  </View>;
}
