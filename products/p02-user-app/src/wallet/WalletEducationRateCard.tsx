import {useState} from 'react';
import {Pressable, StyleSheet, Text, View} from 'react-native';
import type {educationRate} from './walletEducationRate.ts';

export function WalletEducationRateCard({rate, stale, language}: {
  rate: NonNullable<ReturnType<typeof educationRate>>;
  stale: boolean;
  language: 'ko' | 'en';
}) {
  const [open,setOpen] = useState(false);
  const line = (ko:string,en:string) => language === 'ko' ? ko : en;
  const label = open ? line('환산 시세 닫기','Close example rate') : stale ?
    line('마지막 환산 시세','Last example rate') : line('환산 시세','Example rate');
  return <View style={styles.card}>
    <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{expanded:open}}
      onPress={()=>setOpen(!open)} style={styles.button}><Text style={styles.buttonText}>{label}</Text></Pressable>
    {open && <View accessibilityLiveRegion="polite">
      <Text style={styles.text}>{stale ? line('마지막으로 표시한 예시 환산','Last example conversion') :
        line('예시 환산','Example conversion')} ₩{rate.krw.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}</Text>
      <Text style={styles.text}>{rate.amount} TestUSDC · KRW</Text>
      <Text style={styles.text}>1 TestUSDC = 1,300 KRW · {line('교육용 고정 시세 v1','Fixed training rate v1')}</Text>
      <Text style={styles.text}>{line('기준 시각','Reference time')} · {rate.basis}</Text>
      {stale && <Text style={styles.text}>{line('캐시 시각','Cached at')} · {new Date(rate.time).toISOString()}</Text>}
      <Text style={styles.text}>{line('표시 연습용입니다. 교환 또는 현금 상환에 사용할 수 없습니다.',
        'For display practice only. This rate cannot be used for an exchange or cash redemption.')}</Text>
    </View>}
  </View>;
}
const styles = StyleSheet.create({
  card:{gap:6}, text:{fontSize:16,color:'#111'},
  button:{padding:11,backgroundColor:'#123f6b',borderRadius:8}, buttonText:{fontSize:16,color:'#fff',fontWeight:'600'},
});
