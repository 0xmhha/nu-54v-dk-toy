import React from 'react';
import {Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle} from 'react-native';
import {color, font, radius, space, type} from './theme.ts';

// Shared building blocks that carry the design book's look: brand header, status chip, card,
// label/value row, section label, buttons in three weights and the bottom tab bar.

export type ButtonVariant = 'primary' | 'secondary' | 'row';

/** Primary is the one main action of a screen, secondary sits beside it, row is a list entry. */
export function KitButton({label, run, disabled = false, variant = 'primary', testID, detail}: {
  label: string; run: () => void; disabled?: boolean; variant?: ButtonVariant; testID?: string; detail?: string;
}) {
  return <Pressable testID={testID} accessibilityRole="button" accessibilityLabel={label} accessibilityHint={detail} accessibilityState={{disabled}}
    disabled={disabled} onPress={run}
    style={({pressed}) => [styles.button, styles[variant], disabled && styles.disabled, pressed && !disabled && styles.pressed]}>
    <View style={styles.buttonBody}>
      <Text style={[styles.buttonLabel, variant === 'primary' && styles.primaryLabel, variant === 'row' && styles.rowLabel]}>{label}</Text>
      {!!detail && <Text style={styles.detail}>{detail}</Text>}
    </View>
    {variant === 'row' && <View style={styles.chevron} accessibilityElementsHidden importantForAccessibility="no" />}
  </Pressable>;
}

export function Brand({chip, onChip}: {chip?: {label: string; tone: 'ok' | 'wait' | 'off'}; onChip?: () => void}) {
  return <View style={styles.brandRow}>
    <View style={styles.brand} accessibilityRole="header" accessible accessibilityLabel="NU Wallet">
      <View style={styles.mark}><Text style={styles.markText}>n</Text></View>
      <Text style={styles.brandText}>NU Wallet</Text>
    </View>
    {chip && <Chip label={chip.label} tone={chip.tone} onPress={onChip} />}
  </View>;
}

/** Title of a tab's first page with the way into settings, as on every tab root. */
export function TabHeader({title, settings, label = '설정', headingRef}: {
  title: string; settings: () => void; label?: string; headingRef?: React.Ref<React.ComponentRef<typeof Text>>;
}) {
  return <View style={styles.tabHeader}>
    <Text ref={headingRef} accessibilityRole="header" style={[type.title, styles.tabTitle]}>{title}</Text>
    <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={settings} style={({pressed}) => [styles.gear, pressed && styles.pressed]}>
      <View style={iconStyles.gear}>
        {[0, 45, 90, 135].map(turn => <View key={turn} style={[iconStyles.spoke, {transform: [{rotate: `${turn}deg`}]}]} />)}
        <View style={iconStyles.hub} />
      </View>
    </Pressable>
  </View>;
}

export function Chip({label, tone = 'off', onPress}: {label: string; tone?: 'ok' | 'wait' | 'off'; onPress?: () => void}) {
  const body = <View style={[styles.chip, styles[`chip_${tone}`]]}>
    <View style={[styles.dot, styles[`dot_${tone}`]]} />
    <Text style={[styles.chipText, styles[`chipText_${tone}`]]}>{label}</Text>
  </View>;
  return onPress ? <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={onPress}>{body}</Pressable> : body;
}

export function Card({children, tone = 'sheet', style}: {children: React.ReactNode; tone?: 'sheet' | 'green'; style?: StyleProp<ViewStyle>}) {
  return <View style={[styles.card, tone === 'green' && styles.cardGreen, style]}>{children}</View>;
}

export function Section({title, aside, children}: {title: string; aside?: string; children: React.ReactNode}) {
  return <View style={styles.section}>
    <View style={styles.sectionHead}>
      <Text accessibilityRole="header" style={type.section}>{title}</Text>
      {!!aside && <Text style={type.small}>{aside}</Text>}
    </View>
    <View style={styles.group}>{children}</View>
  </View>;
}

export function Row({label, value}: {label: string; value: string}) {
  return <View style={styles.kvRow}><Text style={styles.rowKey}>{label}</Text><Text style={styles.rowValue}>{value}</Text></View>;
}

export type Tab = {key: string; label: string; run: () => void};

/** Drawn tab icons in one stroke weight (no glyphs): house, payment card, wallet, compass, sliders. */
function TabIcon({name, active}: {name: string; active: boolean}) {
  const tint = active ? color.green : color.mute;
  const line = {borderColor: tint, borderWidth: 2};
  if (name === 'home') return <View style={iconStyles.house}><View style={[iconStyles.roof, {borderColor: tint}]} />
    <View style={[iconStyles.walls, {borderColor: tint}]} /></View>;
  if (name === 'wallet') return <View style={[iconStyles.box, line, {borderRadius: 5}]}><View style={[iconStyles.clasp, {borderColor: tint}]} /></View>;
  if (name === 'pay') return <View style={[iconStyles.box, line, {borderRadius: 3}]}><View style={[iconStyles.stripe, {backgroundColor: tint}]} /></View>;
  if (name === 'explore') return <View style={[iconStyles.circle, line]}><View style={[iconStyles.needle, {backgroundColor: tint}]} /></View>;
  return <View style={iconStyles.sliders}>{[6, 14, 9].map((x, i) =>
    <View key={i} style={iconStyles.slider}><View style={[iconStyles.rail, {backgroundColor: tint}]} />
      <View style={[iconStyles.knob, {left: x, borderColor: tint, backgroundColor: active ? color.mint : color.sheet}]} /></View>)}</View>;
}

/** Bottom navigation of the mobile app: 홈 · 결제 · 지갑 · 둘러보기. Settings open from the gear on each tab root. */
export function TabBar({tabs, current, disabled}: {tabs: Tab[]; current: string; disabled?: boolean}) {
  return <View style={[styles.tabs, disabled && styles.tabsOff]} accessibilityRole="tablist">
    {tabs.map(tab => {
      const here = tab.key === current;
      return <Pressable key={tab.key} accessibilityRole="tab" accessibilityLabel={tab.label}
        accessibilityState={{selected: here, disabled}} disabled={disabled} onPress={tab.run} style={styles.tab}>
        <View style={[styles.tabMark, here && styles.tabMarkHere]} />
        {/* A fixed 24x24 box keeps every label on one baseline whatever the icon's own height. */}
        <View style={iconStyles.frame}><TabIcon name={tab.key} active={here} /></View>
        <Text style={[styles.tabText, here && styles.tabTextHere]}>{tab.label}</Text>
      </Pressable>;
    })}
  </View>;
}

const styles = StyleSheet.create({
  button: {minHeight: 50, borderRadius: radius.m, paddingHorizontal: space.l, paddingVertical: space.m,
    flexDirection: 'row', alignItems: 'center', gap: space.s},
  primary: {backgroundColor: color.green},
  secondary: {backgroundColor: color.sheet, borderWidth: 1.5, borderColor: color.green},
  row: {backgroundColor: color.sheet, borderRadius: 0, minHeight: 54},
  disabled: {opacity: 0.45},
  pressed: {opacity: 0.82},
  buttonBody: {flex: 1, gap: 2},
  buttonLabel: {fontFamily: font, fontSize: 16, fontWeight: '700', color: color.green, textAlign: 'center'},
  primaryLabel: {color: '#ffffff'},
  rowLabel: {color: color.ink, fontWeight: '600', textAlign: 'left'},
  detail: {fontFamily: font, fontSize: 13, color: color.mute},
  chevron: {width: 9, height: 9, borderTopWidth: 2, borderRightWidth: 2, borderColor: color.mute, transform: [{rotate: '45deg'}], marginRight: 4},
  tabHeader: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: space.s},
  tabTitle: {flexShrink: 1},
  gear: {width: 48, height: 48, alignItems: 'center', justifyContent: 'center'},
  brandRow: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: space.s},
  brand: {flexDirection: 'row', alignItems: 'center', gap: space.s},
  mark: {width: 34, height: 34, borderRadius: 10, backgroundColor: color.green, alignItems: 'center', justifyContent: 'center'},
  markText: {color: color.mint, fontFamily: font, fontSize: 20, fontWeight: '900', fontStyle: 'italic', marginTop: -3},
  brandText: {fontFamily: font, fontSize: 22, fontWeight: '800', letterSpacing: -0.6, color: color.ink},
  chip: {flexDirection: 'row', alignItems: 'center', gap: 6, borderRadius: radius.pill, paddingHorizontal: 12, paddingVertical: 7},
  chip_ok: {backgroundColor: color.mint}, chip_wait: {backgroundColor: color.amberBg}, chip_off: {backgroundColor: color.line},
  dot: {width: 7, height: 7, borderRadius: 4},
  dot_ok: {backgroundColor: color.green}, dot_wait: {backgroundColor: color.amber}, dot_off: {backgroundColor: color.mute},
  chipText: {fontFamily: font, fontSize: 13, fontWeight: '700'},
  chipText_ok: {color: color.deep}, chipText_wait: {color: color.amber}, chipText_off: {color: color.ink},
  card: {backgroundColor: color.sheet, borderRadius: radius.l, borderWidth: 1, borderColor: color.line, padding: space.l, gap: space.s},
  cardGreen: {backgroundColor: color.green, borderColor: color.green},
  section: {gap: space.s},
  sectionHead: {flexDirection: 'row', alignItems: 'baseline', justifyContent: 'space-between', paddingHorizontal: 2},
  // A 1 px gap over the line color draws the row separators; hairline widths vanish on some densities.
  group: {borderRadius: radius.m, overflow: 'hidden', borderWidth: 1, borderColor: color.line, gap: 1,
    backgroundColor: color.line},
  kvRow: {flexDirection: 'row', justifyContent: 'space-between', gap: space.m, paddingVertical: 6},
  rowKey: {fontFamily: font, fontSize: 15, color: color.mute}, rowValue: {fontFamily: font, fontSize: 15, fontWeight: '700', color: color.ink, flexShrink: 1, textAlign: 'right'},
  tabsOff: {opacity: 0.5},
  tabs: {flexDirection: 'row', borderTopWidth: 1, borderTopColor: color.line, backgroundColor: color.sheet, paddingBottom: 6},
  tab: {flex: 1, alignItems: 'center', paddingTop: 6, paddingBottom: 6, gap: 4, minHeight: 60},
  tabMark: {width: 22, height: 4, borderRadius: 2, backgroundColor: 'transparent'},
  tabMarkHere: {backgroundColor: color.green},
  tabText: {fontFamily: font, fontSize: 13, color: color.mute, fontWeight: '600'},
  tabTextHere: {color: color.green, fontWeight: '800'},
});

const iconStyles = StyleSheet.create({
  frame: {width: 24, height: 24, alignItems: 'center', justifyContent: 'center'},
  box: {width: 22, height: 17, justifyContent: 'center'},
  clasp: {position: 'absolute', right: 2, width: 7, height: 6, borderWidth: 2, borderRadius: 2},
  stripe: {height: 3, marginHorizontal: 0, marginTop: -3},
  // Two borders of a square turned a quarter make the roof above the walls.
  house: {width: 22, height: 20, alignItems: 'center', justifyContent: 'flex-end'},
  roof: {position: 'absolute', top: 3, width: 14, height: 14, borderTopWidth: 2, borderLeftWidth: 2, transform: [{rotate: '45deg'}]},
  walls: {width: 15, height: 10, borderWidth: 2, borderTopWidth: 0, borderRadius: 1},
  // Eight short teeth around a ring: four bars cross behind a ring that hides their middle.
  gear: {width: 24, height: 24, alignItems: 'center', justifyContent: 'center'},
  spoke: {position: 'absolute', width: 5, height: 22, borderRadius: 1.5, backgroundColor: color.ink},
  hub: {width: 16, height: 16, borderRadius: 8, borderWidth: 4.5, borderColor: color.ink, backgroundColor: color.ground},
  circle: {width: 20, height: 20, borderRadius: 10, alignItems: 'center', justifyContent: 'center'},
  needle: {width: 4, height: 10, borderRadius: 2, transform: [{rotate: '40deg'}]},
  sliders: {width: 22, height: 18, justifyContent: 'space-between'},
  slider: {height: 4, justifyContent: 'center'},
  rail: {height: 2, borderRadius: 1},
  knob: {position: 'absolute', top: -2, width: 7, height: 7, borderRadius: 4, borderWidth: 2},
});
