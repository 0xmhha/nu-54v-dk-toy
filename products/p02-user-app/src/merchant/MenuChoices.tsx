import {useRef, useState, type ComponentRef} from 'react';
import {AccessibilityInfo, Pressable, ScrollView, StyleSheet, Text, View} from 'react-native';
import {
  commerceChoiceDisclosure, commerceChoiceState, commerceChooseMessage, commerceLineCapacity, commerceLineKey, commerceMenuLabels,
  commerceOptionLabel, commercePickChange, type CommerceChoiceState, type CommercePhotoCache, type CommercePicking, type CommercePrice, type CommerceSelection,
} from '@nu54/protocol';
import {KitButton} from '../ui/kit.tsx';
import {color, font, radius, space, type} from '../ui/theme.ts';
import {useInnerBack} from '../ui/innerBack.ts';
import {MenuPhoto} from './MenuPhoto.tsx';

export function choicePrice(rows: CommercePrice[], {from, varies}: CommerceChoiceState, locale: 'ko' | 'en', money: (units: string, asset: string) => string) {
  const amount = money(from, rows[0].asset);
  return !varies ? amount : locale === 'ko' ? `${amount}부터` : `From ${amount}`;
}

/** One value is picked in each group. Tapping the picked value takes it back. */
export function MenuChoices({rows, cart, locale, money, disabled, blocked, add, back, photos}: {
  rows: CommercePrice[];
  cart: CommerceSelection;
  locale: 'ko' | 'en';
  money: (units: string, asset: string) => string;
  disabled: boolean;
  /** Why this combination cannot be added to the basket as it is. */
  blocked: (row: CommercePrice) => string | undefined;
  add: (row: CommercePrice, quantity: number) => void;
  back: () => void;
  /** Read only while the phone reaches the server; offline the page shows the name alone. */
  photos?: CommercePhotoCache;
}) {
  const ko = locale === 'ko';
  const [picking, setPicking] = useState<CommercePicking>({picks: {}, quantity: 1});
  const [missing, setMissing] = useState(false);
  const page = useRef<ComponentRef<typeof ScrollView>>(null);
  const heading = useRef<ComponentRef<typeof Text>>(null);
  const top = useRef({list: 0, group: 0});
  useInnerBack(true, back);
  const state = commerceChoiceState(rows, picking.picks, cart);
  if (!state) return null;
  const {row} = state;
  const maximum = row ? commerceLineCapacity(row, cart) : Math.max(...rows.map(item => commerceLineCapacity(item, cart)));
  // The stock may have shrunk under an earlier quantity; adding then asks for what is left.
  const quantity = Math.max(1, Math.min(picking.quantity, maximum));
  const names = state.groups.filter(({picked}) => !picked).map(({labels}) => commerceOptionLabel(labels, locale));
  const first = state.groups.find(({picked}) => !picked)?.key;
  const reason = row && blocked(row);
  const price = row ? money((BigInt(row.unitPrice) * BigInt(quantity)).toString(), row.asset) : choicePrice(rows, state, locale, money);
  const word = !maximum ? (ko ? '품절' : 'Sold out') : row ? (ko ? '담기' : 'Add') : ko ? '골라 주세요' : 'Choose first';
  const submit = () => {
    if (row) return add(row, quantity);
    setMissing(true);
    AccessibilityInfo.announceForAccessibility(commerceChooseMessage(names, locale));
    requestAnimationFrame(() => {
      page.current?.scrollTo({y: top.current.list + top.current.group, animated: false});
      if (heading.current) AccessibilityInfo.sendAccessibilityEvent(heading.current, 'focus');
    });
  };
  return <View style={styles.page}>
    <ScrollView ref={page} style={styles.page} contentContainerStyle={styles.content}>
      <View style={styles.head}>
        <Pressable accessibilityRole="button" accessibilityLabel={ko ? '메뉴로 돌아가기' : 'Back to menu'} onPress={back} style={styles.back}>
          <View style={styles.arrow} />
        </Pressable>
        <Text accessibilityRole="header" style={[type.title, styles.title]}>{commerceMenuLabels(rows[0], locale).product}</Text>
      </View>
      <View style={styles.declared}>
        {photos && rows[0].photo && <MenuPhoto cache={photos} name={rows[0].photo.thumb} height={150} width={200}
          label={`${commerceMenuLabels(rows[0], locale).product} ${ko ? '메뉴 사진' : 'photo'}`} />}
        <Text style={type.body}>{commerceChoiceDisclosure(rows, picking.picks, locale)}</Text>
        {rows.some(({metadata}) => metadata?.allergens?.length || metadata?.crossContact === 'possible') && <Text style={type.small}>
          {ko ? '알레르기가 있으면 주문 전에 직원에게 제조 환경을 확인하세요.' : 'If you have an allergy, ask staff about preparation before ordering.'}</Text>}
      </View>
      <View style={styles.groups} onLayout={event => { top.current.list = event.nativeEvent.layout.y; }}>
        {state.groups.map(({key, labels, picked, values}) => {
          const name = commerceOptionLabel(labels, locale);
          return <View key={key} style={styles.group} onLayout={key === first ? event => { top.current.group = event.nativeEvent.layout.y; } : undefined}>
            <Text ref={key === first ? heading : undefined} accessible accessibilityRole="header" style={type.section}>{name}</Text>
            {missing && !picked && <Text accessibilityRole="alert" style={styles.alert}>{commerceChooseMessage([name], locale)}</Text>}
            <View style={styles.values}>
              {values.map(value => {
                const label = commerceOptionLabel(value.labels, locale);
                const note = value.soldOut === 'always' ? (ko ? '품절' : 'Sold out')
                  : value.soldOut ? (ko ? '지금 고른 것과는 품절' : 'Sold out with this choice')
                  : value.extra > 0n ? `+${money(value.extra.toString(), rows[0].asset)}` : '';
                const off = disabled || (!!value.soldOut && !value.picked);
                return <Pressable key={value.key} accessibilityRole="button" disabled={off}
                  accessibilityLabel={[name, label, note].filter(Boolean).join(' ')} accessibilityState={{selected: value.picked, disabled: off}}
                  onPress={() => setPicking(now => commercePickChange(rows, now, key, value.key, cart))}
                  style={({pressed}) => [styles.value, value.picked && styles.picked, !!value.soldOut && !value.picked && styles.out, pressed && styles.pressed]}>
                  <Text style={[styles.label, value.picked && styles.onPicked, off && styles.outText]}>{value.picked ? `✓ ${label}` : label}</Text>
                  {!!note && <Text style={[styles.note, value.picked && styles.onPicked]}>{note}</Text>}
                </Pressable>;
              })}
            </View>
          </View>;
        })}
      </View>
      {!!reason && <Text accessibilityRole="alert" style={styles.alert}>{reason}</Text>}
    </ScrollView>
    <View style={styles.bar}>
      <View style={styles.steps}>
        <Step sign="−" label={ko ? '수량 줄이기' : 'Decrease quantity'} disabled={disabled || quantity <= 1}
          run={() => setPicking({...picking, quantity: quantity - 1})} />
        <Text accessibilityLiveRegion="polite" accessibilityLabel={`${ko ? '수량' : 'Quantity'} ${quantity}`} style={styles.quantity}>{quantity}</Text>
        <Step sign="+" label={ko ? '수량 늘리기' : 'Increase quantity'} disabled={disabled || quantity >= maximum}
          run={() => setPicking({...picking, quantity: quantity + 1})} />
      </View>
      <View style={styles.add}><KitButton label={`${word} · ${price}`} disabled={disabled || !maximum || !!reason} run={submit} /></View>
    </View>
  </View>;
}

function Step({sign, label, disabled, run}: {sign: string; label: string; disabled: boolean; run: () => void}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{disabled}} disabled={disabled} onPress={run}
    style={[styles.step, disabled && styles.faded]}><Text style={styles.sign}>{sign}</Text></Pressable>;
}

/** The menu's card of a product that is opened to pick. Its combinations in the basket follow below it. */
export function ChoiceCard({rows, cart, locale, money, pick}: {
  rows: CommercePrice[]; cart: CommerceSelection; locale: 'ko' | 'en'; money: (units: string, asset: string) => string;
  /** Absent while nothing can be ordered. */
  pick?: {run: () => void; disabled: boolean};
}) {
  const state = commerceChoiceState(rows, {}, cart);
  if (!state) return null;
  const ko = locale === 'ko';
  const {product} = commerceMenuLabels(rows[0], locale);
  const left = rows.some(row => commerceLineCapacity(row, cart) > (cart[commerceLineKey(row)]?.quantity ?? 0));
  return <View style={styles.card}>
    <Text style={type.label}>{`${product} · ${choicePrice(rows, state, locale, money)}`}</Text>
    <Text style={type.small}>{commerceChoiceDisclosure(rows, {}, locale)}</Text>
    {pick && !left && <Text style={type.body}>{ko ? '품절' : 'Sold out'}</Text>}
    {pick && <KitButton label={ko ? `${product} 옵션 고르기` : `Choose options for ${product}`} disabled={pick.disabled || !left} run={pick.run} />}
  </View>;
}

const styles = StyleSheet.create({
  page: {flex: 1},
  content: {gap: space.l, paddingBottom: space.xl},
  head: {flexDirection: 'row', alignItems: 'center', gap: space.s},
  back: {width: 48, height: 48, alignItems: 'center', justifyContent: 'center'},
  arrow: {width: 13, height: 13, borderBottomWidth: 2.5, borderLeftWidth: 2.5, borderColor: color.ink, transform: [{rotate: '45deg'}], marginLeft: 5},
  title: {flexShrink: 1, fontSize: 22},
  declared: {gap: space.xs},
  groups: {gap: space.l},
  group: {gap: space.s},
  alert: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.danger, backgroundColor: color.dangerBg, borderRadius: radius.m, paddingHorizontal: 14, paddingVertical: 10},
  values: {flexDirection: 'row', flexWrap: 'wrap', gap: space.s},
  value: {flexGrow: 1, flexBasis: '46%', minHeight: 56, borderRadius: radius.m, borderWidth: 1.5, borderColor: '#8fa497', backgroundColor: color.sheet,
    paddingHorizontal: 14, paddingVertical: 10, flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', columnGap: 8},
  picked: {backgroundColor: color.green, borderColor: color.green},
  out: {backgroundColor: '#eef1ea', borderStyle: 'dashed'},
  pressed: {opacity: 0.82},
  faded: {opacity: 0.45},
  label: {fontFamily: font, fontSize: 16, fontWeight: '700', color: color.ink},
  note: {fontFamily: font, fontSize: 14, color: color.mute},
  onPicked: {color: color.onGreen},
  outText: {color: color.mute},
  bar: {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: space.s, paddingVertical: space.s, borderTopWidth: 1, borderTopColor: color.line, backgroundColor: color.ground},
  steps: {flexDirection: 'row', alignItems: 'center'},
  step: {width: 48, height: 48, borderRadius: radius.m, borderWidth: 1.5, borderColor: '#8fa497', backgroundColor: color.sheet, alignItems: 'center', justifyContent: 'center'},
  sign: {fontFamily: font, fontSize: 20, fontWeight: '700', color: color.green},
  quantity: {minWidth: 40, textAlign: 'center', fontFamily: font, fontSize: 18, fontWeight: '800', color: color.ink},
  add: {flexGrow: 1, flexBasis: 180},
  card: {backgroundColor: color.sheet, borderWidth: 1, borderColor: color.line, borderRadius: radius.l, padding: space.l, gap: space.s},
});
