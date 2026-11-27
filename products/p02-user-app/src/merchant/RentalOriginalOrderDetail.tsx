import {Text, View} from 'react-native';

export interface RentalOriginalPayment {
  origin:string;
  amount:string;
  asset:string;
}

export function RentalOriginalOrderDetail({language,order,token,back}:{
  language:'ko'|'en';
  order:RentalOriginalPayment;
  token?:{decimals:number;symbol:string};
  back:React.ReactNode;
}){
  const t=(ko:string,en:string)=>language==='ko'?ko:en;
  const scaled=token ? order.amount.padStart(token.decimals+1,'0') : '';
  const formatted=token ? token.decimals===0 ? scaled :
    scaled.slice(0,-token.decimals)+'.'+scaled.slice(-token.decimals) : '';
  return <View style={{gap:8}}>
    <Text accessibilityRole="header">{t('원래 미결 지급','Original pending payment')}</Text>
    <Text>{t('원거래 참조','Original order reference')} · {order.origin}</Text>
    {token&&<Text>{t('금액','Amount')} · {formatted} {token.symbol}</Text>}
    <Text>{t('금액 (최소 단위)','Amount (base units)')} · {order.amount}</Text>
    <Text>{t('토큰 주소','Token address')} · {order.asset}</Text>
    {back}
  </View>;
}
