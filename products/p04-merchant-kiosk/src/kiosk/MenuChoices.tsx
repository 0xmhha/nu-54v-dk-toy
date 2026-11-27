import type { ComponentRef, RefObject } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { commerceChooseMessage, commerceOptionLabel, type CommerceChoiceState } from '@nu54/protocol';

/** One value is picked in each group. Tapping the picked value takes it back. */
export function MenuChoices({ state, locale, money, missing, pick, heading, place }: {
  state: CommerceChoiceState;
  locale: 'ko' | 'en';
  money: (units: string) => string;
  /** The customer tried to add before every group was picked. */
  missing: boolean;
  pick: (group: string, value: string) => void;
  /** The heading of the first group that is not picked. */
  heading: RefObject<ComponentRef<typeof Text> | null>;
  place: (top: number) => void;
}) {
  const ko = locale === 'ko';
  const first = state.groups.find(({ picked }) => !picked)?.key;
  return <View style={styles.groups}>
    {state.groups.map(({ key, labels, picked, values }) => {
      const name = commerceOptionLabel(labels, locale);
      return <View key={key} style={styles.group}
        onLayout={key === first ? event => place(event.nativeEvent.layout.y) : undefined}>
        <Text ref={key === first ? heading : undefined} accessible accessibilityRole="header" style={styles.name}>{name}</Text>
        {missing && !picked && <Text accessibilityRole="alert" style={styles.alert}>{commerceChooseMessage([name], locale)}</Text>}
        <View style={styles.values}>
          {values.map(value => {
            const label = commerceOptionLabel(value.labels, locale);
            const note = value.soldOut === 'always' ? (ko ? '품절' : 'Sold out')
              : value.soldOut ? (ko ? '지금 고른 것과는 품절' : 'Sold out with this choice')
              : value.extra > 0n ? `+${money(value.extra.toString())}` : '';
            const disabled = !!value.soldOut && !value.picked;
            return <Pressable key={value.key} accessibilityRole="button" disabled={disabled}
              accessibilityLabel={[name, label, note].filter(Boolean).join(' ')}
              accessibilityState={{ selected: value.picked, disabled }}
              onPress={() => pick(key, value.key)}
              style={({ pressed }) => [styles.value, value.picked && styles.picked, disabled && styles.out, pressed && styles.pressed]}>
              <Text style={[styles.label, value.picked && styles.onPicked, disabled && styles.outText]}>{value.picked ? `✓ ${label}` : label}</Text>
              {!!note && <Text style={[styles.note, value.picked && styles.onPicked, disabled && styles.outText]}>{note}</Text>}
            </Pressable>;
          })}
        </View>
      </View>;
    })}
  </View>;
}

const styles = StyleSheet.create({
  groups: { gap: 18 },
  group: { gap: 8 },
  name: { fontSize: 20, fontWeight: '800', color: '#182d25' },
  alert: { fontSize: 17, lineHeight: 24, color: '#7a2020', backgroundColor: '#f8e1df', borderRadius: 12, paddingHorizontal: 14, paddingVertical: 10 },
  values: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  value: {
    flexGrow: 1, flexBasis: '30%', minWidth: 150, minHeight: 64, borderRadius: 16, borderWidth: 2, borderColor: '#8fa497', backgroundColor: '#fffefa',
    paddingHorizontal: 18, paddingVertical: 12, flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', columnGap: 10,
  },
  picked: { backgroundColor: '#174b38', borderColor: '#174b38' },
  out: { backgroundColor: '#eef1ea', borderStyle: 'dashed' },
  pressed: { opacity: 0.75 },
  label: { fontSize: 18, fontWeight: '700', color: '#182d25' },
  note: { fontSize: 16, color: '#4f655a' },
  onPicked: { color: '#f4f8ed' },
  outText: { color: '#4f655a' },
});
