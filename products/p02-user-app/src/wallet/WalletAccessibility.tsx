import React, {type ComponentRef, useEffect, useRef, useState} from 'react';
import {AccessibilityInfo, findNodeHandle, Pressable, StyleSheet, Text, View} from 'react-native';
import {practiceOrderText} from './practiceFlow.ts';

type Panel = 'home' | 'font' | 'large' | 'reader' | 'approval' | 'notification';

export function WalletAccessibility({language, reader, motion, notification, setLanguage, openDeviceSettings, practice, pendingOrders, rejectPractice, requestNotifications, requestingNotifications, resetAccessibilityScroll}: {
  language: 'ko' | 'en';
  reader?: boolean;
  motion?: boolean;
  notification?: boolean;
  setLanguage: (language: 'ko' | 'en') => void;
  openDeviceSettings: () => void;
  practice: () => void;
  pendingOrders: () => void;
  rejectPractice?: () => void;
  requestNotifications?: () => void;
  requestingNotifications: boolean;
  resetAccessibilityScroll?: () => void;
}) {
  const ko = language === 'ko';
  const [panel, setPanel] = useState<Panel>('home');
  const panelHeading = useRef<ComponentRef<typeof Text>>(null);
  useEffect(() => {resetAccessibilityScroll?.();}, [panel, resetAccessibilityScroll]);
  useEffect(() => {
    if (!reader) return;
    const timer = setTimeout(() => {
      const node = findNodeHandle(panelHeading.current);
      if (node) AccessibilityInfo.setAccessibilityFocus(node);
    }, 200);
    return () => clearTimeout(timer);
  }, [panel, reader]);
  const order = practiceOrderText(language);
  const rows = [
    ['source', ko ? '요청 출처' : 'Request source', ko ? '보드 없는 연습 모드' : 'Practice without a board'],
    ['amount', ko ? '금액' : 'Amount', order.amount],
    ['recipient', ko ? '수취인' : 'Recipient', order.recipient],
    ['chain', ko ? '체인' : 'Network', order.chain],
    ['fee', ko ? '연습 수수료' : 'Practice fee', order.fee],
    ['method', ko ? '확인 방법' : 'Review method', ko ? '가상 버튼 연습' : 'Virtual button practice'],
    ['cancel', ko ? '마지막 행동' : 'Last action', ko ? '요청 취소' : 'Cancel request'],
  ].map(([id, label, value]) => ({id, text: label + ': ' + value}));

  const button = (label: string, action: () => void, large = false, disabled = false) =>
    <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{disabled}} disabled={disabled} onPress={action} style={styles.button}>
      <Text style={[styles.buttonText, large && styles.largeButtonText]}>{label}</Text>
    </Pressable>;
  const heading = (text: string) => <Text ref={panelHeading} accessibilityRole="header" style={styles.heading}>{text}</Text>;
  const goBack = () => button(ko ? '접근성으로' : 'Back to accessibility', () => setPanel('home'));
  const onOff = (value?: boolean) => value === undefined
    ? (ko ? '확인할 수 없음' : 'Unavailable')
    : value ? (ko ? '켜짐' : 'On') : (ko ? '꺼짐' : 'Off');
  if (panel === 'font' || panel === 'large') {
    const large = panel === 'large';
    const textStyle = large ? styles.large : styles.text;
    return <View style={styles.group}>
      {heading(ko ? '결제 내용을 큰 글씨로 보기' : 'Preview payment with large text')}
      <Text style={styles.text}>{ko
        ? '이 미리보기에서만 글자를 바꿉니다. 앱의 실제 설정은 기기 설정을 따릅니다.'
        : 'Text changes here apply to the preview. The app uses your device settings.'}</Text>
      <Text testID="w27-practice-amount" style={textStyle}>{order.amount}</Text>
      <Text style={textStyle}>{rows[2].text}</Text>
      <Text style={textStyle}>{rows[3].text}</Text>
      {large ? <>
        <Text style={styles.text}>{ko ? '동작 줄이기 · 보드 버튼 정적 안내' : 'Reduced motion · Static board button guide'}</Text>
        <Text style={styles.guide}>{ko ? '보드 그림 오른쪽 · SW1 확인 / SW2 거절' : 'Right side of the board · SW1 Confirm / SW2 Decline'}</Text>
        {rejectPractice && button(ko ? '연습 결제 취소' : 'Cancel practice payment', rejectPractice, true)}
        {button(ko ? '기본 글자 선택' : 'Use default text', () => setPanel('font'))}
      </> : button(ko ? '200%와 동작 줄이기 미리보기' : 'Preview 200% and reduced motion', () => setPanel('large'))}
      {goBack()}
    </View>;
  }

  if (panel === 'reader') return <View style={styles.group}>
    {heading(ko ? '연습 주문을 순서대로 듣기' : 'Hear the practice order in sequence')}
    <Text style={styles.text}>{ko ? 'VoiceOver 또는 TalkBack에서 다음 항목으로 이동하세요.' : 'Use VoiceOver or TalkBack to move to the next item.'}</Text>
    {rows.map(row => <Text key={row.id} testID={'w27-read-' + row.id} accessible style={styles.text}>{row.text}</Text>)}
    {button(ko ? '안내 다시 듣기' : 'Hear again', () => AccessibilityInfo.announceForAccessibility(rows.map(row => row.text).join('. ')))}
    {rejectPractice && button(ko ? '거절 연습' : 'Decline practice', rejectPractice)}
    {button(ko ? '실제 기기 승인 화면 예시' : 'Real approval screen example', () => setPanel('approval'))}
    {goBack()}
  </View>;

  if (panel === 'approval') return <View style={styles.group}>
    {heading(ko ? '실제 기기 승인 화면 예시' : 'Real approval screen example')}
    <Text style={styles.text}>{ko
      ? '실물 보드가 필요합니다. 인증된 견적의 금액·수취인·체인을 독립 표시에서 확인하세요. 맞으면 보드 SW1, 다르면 SW2를 누르세요.'
      : 'Requires the physical board. Compare the authenticated quote amount, recipient and network on the independent display. Press SW1 to confirm or SW2 to decline.'}</Text>
    <View style={styles.guide}>
      <Text style={styles.text}>{ko ? '예시 · 인증된 가맹점 견적' : 'Example · Authenticated merchant quote'}</Text>
      <Text style={styles.text}>{ko ? '금액: 6.50 TestUSDC' : 'Amount: 6.50 TestUSDC'}</Text>
      <Text style={styles.text}>{ko ? '수취인: 여울 커피 성수점' : 'Recipient: Yeoul Coffee Seongsu'}</Text>
      <Text style={styles.text}>{ko ? '네트워크: StableNet 테스트넷 8283' : 'Network: StableNet testnet 8283'}</Text>
      <Text style={styles.text}>{ko ? '가스 비용: 매장 부담' : 'Gas fee: paid by the store'}</Text>
    </View>
    {button(ko ? '화면 읽기 순서로' : 'Back to screen reader order', () => setPanel('reader'))}
    {goBack()}
  </View>;

  if (panel === 'notification') return <View style={styles.group}>
    {heading(ko ? '알림에 표시할 정보' : 'Lock screen notification preview')}
    <Text style={styles.text}>{ko ? '예시 · 민감한 값 숨김' : 'Example · Sensitive details hidden'}</Text>
    <Text testID="w27-lock-notification" style={styles.guide}>{ko ? '확인할 결제가 있습니다' : 'A payment needs review'}</Text>
    <Text style={styles.text}>{ko
      ? '잠금 화면 미리보기에는 주소·금액·상점명이 없습니다. 앱을 열면 원래 주문을 조회할 수 있는지 확인합니다.'
      : 'The preview contains no address, amount or merchant. The app checks access to the original order after opening.'}</Text>
    <Text style={styles.text}>{ko ? '기기 알림 권한' : 'Device notification permission'}: {
      notification === false ? (ko ? '알림 예약 불가' : 'Scheduling unavailable') : onOff(notification)
    }</Text>
    {notification === false && <Text style={styles.text}>{ko ? '결제 요청을 앱 안에서 확인하세요.' : 'Review payment requests in the app.'}</Text>}
    <Text style={styles.text}>{ko
      ? '알림을 꺼도 청구와 지급 상태는 유지됩니다.'
      : 'Turning off notifications leaves billing and payment status unchanged.'}</Text>
    {requestNotifications && button(ko ? '알림 허용 요청' : 'Request notification permission', requestNotifications, false, requestingNotifications)}
    {button(ko ? '기기에서 알림 설정' : 'Manage notifications in device settings', openDeviceSettings)}
    {button(ko ? '미결 목록' : 'Pending payment list', pendingOrders)}
    {goBack()}
  </View>;

  return <View style={styles.group}>
    {heading(ko ? '표시와 알림을 설정하세요' : 'Display and notification settings')}
    <Text style={styles.text}>{ko ? '화면 읽기' : 'Screen reader'}: {onOff(reader)} · {ko ? '동작 줄이기' : 'Reduce motion'}: {onOff(motion)}</Text>
    <Text style={styles.text}>{ko ? '결제 승인은 원래 견적 기한을 따릅니다.' : 'Payment approval uses the original quote deadline.'}</Text>
    {button(ko ? '글자와 움직임' : 'Text and motion', () => setPanel('font'))}
    {button(ko ? '화면 읽기 순서' : 'Screen reader order', () => setPanel('reader'))}
    <Text style={styles.text}>{ko ? '표시 언어: 한국어' : 'Display language: English'}</Text>
    {button(ko ? 'English 선택' : '한국어 선택', () => setLanguage(ko ? 'en' : 'ko'))}
    {button(ko ? '알림 미리보기' : 'Notification preview', () => setPanel('notification'))}
    {button(ko ? '기기 설정 열기' : 'Open device settings', openDeviceSettings)}
    {button(ko ? '결제 연습 열기' : 'Open payment practice', practice)}
  </View>;
}

const styles = StyleSheet.create({
  group: {gap: 12},
  heading: {fontSize: 22, fontWeight: '700', color: '#111'},
  text: {fontSize: 17, color: '#111'},
  large: {fontSize: 34, color: '#111'},
  guide: {fontSize: 17, color: '#111', backgroundColor: '#eee', padding: 12},
  button: {padding: 14, minHeight: 48, backgroundColor: '#193d33', borderRadius: 9},
  buttonText: {fontSize: 17, color: '#fff', fontWeight: '600'},
  largeButtonText: {fontSize: 34},
});
